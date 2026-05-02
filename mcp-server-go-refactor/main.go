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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"mcp-server-go-refactor/internal/config"
	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/external"
	"mcp-server-go-refactor/internal/handler"
	"mcp-server-go-refactor/internal/handler/system"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/runner"
	"mcp-server-go-refactor/internal/server"
	"mcp-server-go-refactor/internal/staging"
	"mcp-server-go-refactor/internal/util"

	"github.com/tidwall/buntdb"
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
	cleanupLogs := util.SetupStandardLogging("go-refactor", buffer)

	defer cleanupLogs()

	slog.Info("[LIFECYCLE] SPAWN "+config.Name, "version", Version)

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
	// Provision the standalone Go toolchain to ensure independence from host OS.
	goBin, err := runner.EnsureToolchain(ctx, "1.26.2")
	if err != nil {
		slog.Warn("failed to provision isolated Go toolchain; attempting graceful degradation", "error", err)
	} else {
		runner.DefaultGoBinary = goBin
	}

	// Initialize persistent cache using platform-native cache directory.
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("get user cache directory: %w", err)
	}
	appCacheDir := filepath.Join(cacheDir, "mcp-server-go-refactor")
	if err := os.MkdirAll(appCacheDir, 0o750); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}
	dbPath := filepath.Join(appCacheDir, "cache.db")
	db, err := buntdb.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open buntdb: %w", err)
	}
	var bconfig buntdb.Config
	if err := db.ReadConfig(&bconfig); err == nil {
		bconfig.SyncPolicy = buntdb.EverySecond
		bconfig.AutoShrinkPercentage = 50
		bconfig.AutoShrinkMinSize = 25 * 1024 * 1024
		if setErr := db.SetConfig(bconfig); setErr != nil {
			slog.Warn("failed to apply buntdb cache configuration", "error", setErr)
		}
	} else {
		slog.Warn("failed to read buntdb cache configuration", "error", err)
	}

	if err := staging.SetupIndexes(db); err != nil {
		slog.Warn("failed to setup staging indexes", "error", err)
	}

	defer db.Close()

	eng := engine.NewEngine(db)
	establishStreamingClient(ctx, eng)

	handler.RegisterAllTools(eng, buffer)
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
				hits, misses, memEntries := loader.GetPackageCacheMetrics()
				dbEntries := eng.DBEntries()
				slog.Info("cache metrics",
					"hits", hits,
					"misses", misses,
					"mem_entries", memEntries,
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
			if err := waitForRecallSocketReady(slog.Default()); err != nil {
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
