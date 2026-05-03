// Package main provides functionality for the main subsystem.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
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

	"mcp-server-brainstorm/internal/persistent"

	"mcp-server-brainstorm/internal/config"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/external"
	"mcp-server-brainstorm/internal/handler"
	"mcp-server-brainstorm/internal/handler/system"
	"mcp-server-brainstorm/internal/server"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
)

var exitFunc = os.Exit

func main() {
	// Defense-in-depth: Unmanaged Standalone Fallbacks
	if _, exists := os.LookupEnv("GOMEMLIMIT"); !exists {
		os.Setenv("GOMEMLIMIT", "1024MiB")
	}
	if _, exists := os.LookupEnv("GOMAXPROCS"); !exists {
		os.Setenv("GOMAXPROCS", "2")
	}
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		printVersion()
		exitFunc(0)
	}

	// Resource tuning is now orchestrated by magictools via GOMEMLIMIT and GOMAXPROCS
	realStdout := os.Stdout
	os.Stdout = os.Stderr

	buffer := &system.LogBuffer{}
	cleanupLogs := util.SetupStandardLogging("brainstorm", buffer)

	defer cleanupLogs()

	slog.Info("[LIFECYCLE] SPAWN "+config.Name, "version", Version)

	//nolint:contextcheck // main is the root of the context boundary
	rootCtx := context.Background()
	ctx, stop := signal.NotifyContext(rootCtx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	reader := bufio.NewReaderSize(os.Stdin, 128*1024)
	writer := bufio.NewWriterSize(realStdout, 128*1024)

	if err := run(ctx, stop, buffer, reader, writer); err != nil {
		if isExpectedShutdownErr(err) {
			slog.Info("server shut down gracefully", "error", err)
			if ferr := writer.Flush(); ferr != nil {
				slog.Debug("failed to flush real stdout on shutdown", "error", ferr)
			}
			exitFunc(0)
			return
		}
		slog.Error("server fatal error", "error", err)
		exitFunc(1)
	}
	if err := writer.Flush(); err != nil {
		slog.Debug("final flush failed", "error", err)
	}
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

func run(ctx context.Context, cancel context.CancelFunc, buffer *system.LogBuffer, reader io.Reader, writer io.Writer) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Initialize persistent cache using platform-native cache directory.
	db, err := persistent.OpenDB("mcp-server-brainstorm")
	if err != nil {
		return err
	}
	defer db.Close()

	eng := engine.NewEngine(wd, db)
	if config.IsOrchestratorOwned() {
		establishStreamingClient(ctx, eng)
	}

	mgr := state.NewManager(wd)
	handler.RegisterAllTools(mgr, eng, buffer)
	mcpServer := server.NewMCPServer(config.Platform+" Analyzer", Version, slog.Default())
	count := handler.LoadToolsFromRegistry(mcpServer)
	slog.Info("all tools registered with SDK", "count", count)

	// Metrics reporter: log cache performance every 30 seconds.
	go func(c context.Context) {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-c.Done():
				return
			case <-ticker.C:
				dbEntries := eng.DBEntries()
				slog.Info("cache metrics",
					"hits", 0,
					"misses", 0,
					"mem_entries", 0,
					"db_entries", dbEntries,
				)
			}
		}
	}(ctx)

	// Synchronization guard: small delay to ensure SDK state is flushed
	// before the orchestrator's initial sync.
	time.Sleep(100 * time.Millisecond)

	errChan := make(chan error, 1)
	go func(c context.Context) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("CRITICAL: Server panic recovered in main run loop", "recover", r)
				errChan <- fmt.Errorf("critical panic: %v", r)
			}
		}()
		eofReader := &eofDetector{
			r:      reader,
			cancel: cancel,
		}
		autoWriter := &autoFlusher{w: writer}
		if err := mcpServer.Serve(c, util.NopWriteCloser{Writer: autoWriter}, util.NopReadCloser{Reader: eofReader}); err != nil {
			errChan <- err
		}
	}(ctx)

	select {
	case <-ctx.Done():
		slog.Info("[LIFECYCLE] context cancelled; initiating graceful shutdown")
	case err := <-errChan:
		if isExpectedShutdownErr(err) {
			slog.Info("[LIFECYCLE] stdio transport closed gracefully", "reason", err.Error())
			return nil
		}
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func establishStreamingClient(ctx context.Context, eng *engine.Engine) {
	apiURLs := config.ResolveAPIURLs()
	for _, u := range apiURLs {
		slog.Info("[RECALL] initializing external context client", "url", u)
		client := external.NewMCPClient(u)
		eng.SetExternalClient(client)
		go func(url string, c context.Context) {
			// Start() has its own exponential backoff retry loop (1s → 30s).
			// No boot delay needed — if recall isn't ready, Start() retries.
			if err := waitForRecallSocketReady(); err != nil {
				slog.Error("recall socket wait failed", "error", err)
			}
			client.Start(c)
		}(u, ctx)
	}
}

// eofDetector safely monitors for EOF on Read calls to trigger shutdown
type eofDetector struct {
	r      io.Reader
	cancel context.CancelFunc
}

// Read safely extracts bytes while checking for explicit EOF.
func (e *eofDetector) Read(p []byte) (n int, err error) {
	n, err = e.r.Read(p)
	if errors.Is(err, io.EOF) {
		slog.Warn("[LIFECYCLE] orchestrator pipe closed (EOF); self-terminating")
		e.cancel()
	}
	return n, err
}

type flusher interface {
	Flush() error
}

// autoFlusher ensures the 64KB buffer is flushed immediately after each JSON message
type autoFlusher struct {
	w io.Writer
}

// Write intercepts payload passing and automatically flushes the pipeline.
func (a *autoFlusher) Write(p []byte) (n int, err error) {
	n, err = a.w.Write(p)
	if f, ok := a.w.(flusher); ok {
		if ferr := f.Flush(); ferr != nil {
			slog.Debug("auto-flush failed", "error", ferr)
		}
	}
	return n, err
}

func waitForRecallSocketReady() error {
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

	timeoutRoot := context.Background()
	ctx, cancel := context.WithTimeout(timeoutRoot, 5*time.Minute)
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
