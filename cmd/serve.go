package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"mcp-server-magicskills/internal/config"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/external"
	"mcp-server-magicskills/internal/handler"
	"mcp-server-magicskills/internal/handler/discovery"
	"mcp-server-magicskills/internal/handler/execution"
	"mcp-server-magicskills/internal/handler/retrieval"
	"mcp-server-magicskills/internal/handler/sync"
	"mcp-server-magicskills/internal/handler/system"
	"mcp-server-magicskills/internal/registry"
	"mcp-server-magicskills/internal/scanner"
	"mcp-server-magicskills/internal/server"
	"mcp-server-magicskills/internal/state"
	"mcp-server-magicskills/internal/util"
)

var serveCmd = &cobra.Command{
	Use:   "serve [paths...]",
	Short: "Start the magicskills JSON-RPC MCP server over stdio",
	RunE: func(cmd *cobra.Command, args []string) error {
		HijackStdout()

		rootCtx := context.Background()
		ctx, stop := signal.NotifyContext(rootCtx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		defer stop()

		logBuffer := &handler.LogBuffer{}
		cleanupLogs := util.SetupStandardLogging("magicskills", logBuffer)
		defer cleanupLogs()

		slog.Info("[BACKPLANE] SPAWN mcp-server-magicskills", "version", Version)

		reader := bufio.NewReaderSize(os.Stdin, 128*1024)
		writer := bufio.NewWriterSize(RealStdout, 128*1024)

		if err := execute(ctx, stop, logBuffer, reader, writer, args); err != nil {
			if isExpectedShutdownErr(err) {
				slog.Info("server shut down gracefully", "error", err)
				if flushErr := writer.Flush(); flushErr != nil {
					slog.Warn("failed to flush final stream", "error", flushErr)
				}
				return nil
			}
			slog.Error("server fatal error", "error", err)
			return err
		}
		if flushErr := writer.Flush(); flushErr != nil {
			slog.Warn("failed to flush final stream", "error", flushErr)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	// Default to server mode when no arguments are provided
	rootCmd.RunE = serveCmd.RunE
}

func isExpectedShutdownErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, phrase := range []string{"eof", "broken pipe", "connection reset", "use of closed", "file already closed", "bad file descriptor", "client is closing", "connection closed"} {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}

func initServer(logBuffer *handler.LogBuffer, extraRoots []string) (*state.Store, *engine.Engine, *scanner.Scanner, *handler.MagicSkillsHandler, error) {
	roots := config.ResolveRoots()
	roots = append(roots, scanner.FindProjectSkillsRoots()...)

	for _, arg := range extraRoots {
		if info, err := os.Stat(arg); err == nil && info.IsDir() {
			roots = append(roots, arg)
			slog.Info("added manual skill root", "path", arg)
		}
	}

	dbPath := config.ResolveDataDir()
	store, err := state.NewStore(dbPath)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to init state store: %w", err)
	}

	eng, err := engine.NewEngine(store, dbPath+"/engine_index")
	if err != nil {
		store.Close()
		return nil, nil, nil, nil, fmt.Errorf("engine init: %w", err)
	}

	recallURL := config.ResolveRecallURL()
	cl := external.NewMCPClient(recallURL)

	h := &handler.MagicSkillsHandler{
		Engine:       eng,
		Logs:         logBuffer,
		RecallClient: cl,
	}

	scn, err := scanner.NewScanner(roots)
	if err != nil {
		store.Close()
		return nil, nil, nil, nil, fmt.Errorf("scanner init: %w", err)
	}

	return store, eng, scn, h, nil
}

func initSubsystems(ctx context.Context, eng *engine.Engine, scn *scanner.Scanner, logBuffer *handler.LogBuffer, cl *external.MCPClient) {
	discovery.Register(eng, cl)
	retrieval.Register(eng, cl)
	execution.Register(eng, cl)
	sync.Register(eng, scn)
	system.Register(eng, scn, logBuffer)

	if scn != nil && eng != nil {
		// Background Ingestion
		go func(c context.Context) {
			files, err := scn.Discover(c)
			if err != nil {
				slog.Warn("discovery produced errors", "error", err)
			}
			if _, _, _, err := eng.SyncDir(c, files); err != nil {
				slog.Error("initial ingestion failed", "error", err)
			}
			slog.Info("engine ready", "skillsCount", len(eng.Skills), "version", Version, "rootsCount", len(scn.Roots))
			close(eng.ReadyCh)
		}(ctx)

		// Background Incremental Watcher
		scn.Listen(ctx, func(path string) {
			if err := eng.IngestSingle(ctx, path); err != nil {
				slog.Error("incremental update failed", "path", path, "error", err)
			} else {
				slog.Info("engine cache updated", "path", path)
			}
		}, func(path string) {
			eng.Remove(ctx, path)
			slog.Info("engine cache item removed", "path", path)
		})
	}
}

func execute(ctx context.Context, cancel context.CancelFunc, logBuffer *handler.LogBuffer, reader io.Reader, writer io.Writer, extraRoots []string) error {
	logger := slog.Default()

	store, eng, scn, h, err := initServer(logBuffer, extraRoots)
	if err != nil {
		return err
	}
	defer store.Close()
	defer scn.Watcher.Close()

	initSubsystems(ctx, eng, scn, logBuffer, h.RecallClient)

	if h.RecallClient != nil {
		go func() {
			if err := waitForRecallSocketReady(slog.Default()); err != nil {
				slog.Error("recall socket wait failed", "error", err)
			}
			h.RecallClient.Start(ctx)
		}()
	}

	go func(c context.Context) {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-c.Done():
				return
			case <-ticker.C:
				hits, misses := store.GetMetrics()
				entries := store.CountEntries()
				skills := eng.CountSkills()
				bleveDocs := eng.CountBleveDocs()
				slog.Info("cache metrics",
					"server", "magicskills",
					"hits", hits,
					"misses", misses,
					"efficacy_records", entries,
					"skills", skills,
					"bleve_docs", bleveDocs,
				)
			}
		}
	}(ctx)

	mcpServer := server.NewMCPServer("mcp-server-magicskills", Version, logger)

	for _, t := range registry.Global.List() {
		t.Register(mcpServer.MCPServer())
	}
	h.RegisterResources(mcpServer.MCPServer())

	errChan := make(chan error, 1)
	go func(c context.Context, cFunc context.CancelFunc) {
		eofReader := &eofDetector{r: reader, cancel: cFunc}
		autoWriter := &autoFlusher{w: writer}
		if err := mcpServer.Serve(c, util.NopWriteCloser{Writer: autoWriter}, util.NopReadCloser{Reader: eofReader}); err != nil {
			errChan <- err
		}
	}(ctx, cancel)

	select {
	case <-ctx.Done():
		slog.Info("context cancelled; initiating graceful shutdown")
	case err := <-errChan:
		if isExpectedShutdownErr(err) {
			slog.Info("stdio transport closed gracefully", "reason", err.Error())
			return nil
		}
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

type eofDetector struct {
	r      io.Reader
	cancel context.CancelFunc
}

func (e *eofDetector) Read(p []byte) (n int, err error) {
	n, err = e.r.Read(p)
	if err == io.EOF {
		slog.Warn("orchestrator pipe closed (EOF); self-terminating")
		e.cancel()
	}
	return n, err
}

type flusher interface {
	Flush() error
}

type autoFlusher struct {
	w io.Writer
}

func (a *autoFlusher) Write(p []byte) (n int, err error) {
	n, err = a.w.Write(p)
	if f, ok := a.w.(flusher); ok {
		if flushErr := f.Flush(); flushErr != nil {
			slog.Warn("auto flush failed", "error", flushErr)
		}
	}
	return n, err
}

func waitForRecallSocketReady(logger *slog.Logger) error {
	val := os.Getenv("MCP_API_URL")
	if val == "" {
		// Standalone execution bound; ignore ping.
		return nil
	}

	// Parse first chunk natively ignoring trailing commas
	urls := strings.Split(val, ",")
	u, err := url.Parse(strings.TrimSpace(urls[0]))
	if err != nil || u.Host == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("recall connection timeout threshold exceeded")
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", u.Host, 100*time.Millisecond)
			if err == nil && conn != nil {
				conn.Close()
				return nil
			}
		}
	}
}
