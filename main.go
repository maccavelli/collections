package main

// Pipeline Validation Complete

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"mcp-server-duckduckgo/internal/config"
	"mcp-server-duckduckgo/internal/engine"
	"mcp-server-duckduckgo/internal/handler/media"
	"mcp-server-duckduckgo/internal/handler/search"
	"mcp-server-duckduckgo/internal/handler/system"
	"mcp-server-duckduckgo/internal/registry"
	"mcp-server-duckduckgo/internal/server"
	"mcp-server-duckduckgo/internal/util"
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

	realStdout := os.Stdout
	os.Stdout = os.Stderr

	logBuffer := &system.LogBuffer{}
	cleanupLogs := util.SetupStandardLogging("duckduckgo", logBuffer)
	defer cleanupLogs()

	slog.Info("[BACKPLANE] SPAWN "+config.Name, "version", Version)

	rootCtx := context.Background()
	ctx, stop := signal.NotifyContext(rootCtx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	reader := bufio.NewReaderSize(os.Stdin, 128*1024)
	writer := bufio.NewWriterSize(realStdout, 128*1024)

	if err := run(ctx, stop, logBuffer, reader, writer); err != nil {
		if isExpectedShutdownErr(err) {
			slog.Info("server shut down gracefully", "error", err)
			if err := writer.Flush(); err != nil {
				slog.Error("failed to flush stdout on shutdown", "error", err)
			}
			exitFunc(0)
			return
		}
		slog.Error("server fatal error", "error", err)
		exitFunc(1)
	}
	if err := writer.Flush(); err != nil {
		slog.Error("failed to final flush stdout", "error", err)
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

func run(ctx context.Context, cancel context.CancelFunc, lb *system.LogBuffer, reader io.Reader, writer io.Writer) error {
	eng := engine.NewSearchEngine()
	search.Register(eng)
	media.Register(eng)
	system.Register(lb)

	mcpServer := server.NewMCPServer(config.Name, Version, slog.Default())

	for _, t := range registry.Global.List() {
		t.Register(mcpServer.MCPServer())
	}

	errChan := make(chan error, 1)
	go func() {
		eofReader := &eofDetector{
			r:      reader,
			cancel: cancel,
		}
		autoWriter := &autoFlusher{w: writer}
		if err := mcpServer.Serve(ctx, util.NopWriteCloser{Writer: autoWriter}, util.NopReadCloser{Reader: eofReader}); err != nil {
			errChan <- err
		}
	}()

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

// eofDetector safely monitors for EOF on Read calls to trigger shutdown
type eofDetector struct {
	r      io.Reader
	cancel context.CancelFunc
}

// Read proxies the underlying reader and detects EOF closure from the orchestrator pipe.
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

// autoFlusher ensures the 64KB buffer is flushed immediately after each JSON message
type autoFlusher struct {
	w io.Writer
}

// Write proxies the underlying writer and enforces immediate buffer flushing.
func (a *autoFlusher) Write(p []byte) (n int, err error) {
	n, err = a.w.Write(p)
	if err != nil {
		return n, err
	}
	if f, ok := a.w.(flusher); ok {
		if ferr := f.Flush(); ferr != nil {
			return n, ferr
		}
	}
	return n, nil
}
