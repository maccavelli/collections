package main

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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-sequential-thinking/internal/config"
	"mcp-server-sequential-thinking/internal/engine"
	"mcp-server-sequential-thinking/internal/handler/sequentialthinking"
	"mcp-server-sequential-thinking/internal/handler/system"
	"mcp-server-sequential-thinking/internal/registry"
	"mcp-server-sequential-thinking/internal/server"
	"mcp-server-sequential-thinking/internal/util"
)

var exitFunc = os.Exit

func main() {
	// Defense-in-depth: Unmanaged Standalone Fallbacks
	if _, exists := os.LookupEnv("GOMEMLIMIT"); !exists {
		if err := os.Setenv("GOMEMLIMIT", "1024MiB"); err != nil {
			slog.Warn("failed to set fallback GOMEMLIMIT", "error", err)
		}
	}
	if _, exists := os.LookupEnv("GOMAXPROCS"); !exists {
		if err := os.Setenv("GOMAXPROCS", "2"); err != nil {
			slog.Warn("failed to set fallback GOMAXPROCS", "error", err)
		}
	}
	var versionFlag bool
	flag.BoolVar(&versionFlag, "version", false, "Print version and exit")
	flag.Parse()

	if versionFlag {
		printVersion()
		exitFunc(0)
	}

	realStdout := os.Stdout
	os.Stdout = os.Stderr

	rootCtx := context.Background()
	ctx, stop := signal.NotifyContext(rootCtx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	buffer := &system.LogBuffer{}
	cleanupLogs := util.SetupStandardLogging(ctx, "sequential-thinking", buffer)
	defer cleanupLogs()

	logger := slog.Default()
	slog.Info("[BACKPLANE] SPAWN "+config.Name, "version", Version)

	reader := bufio.NewReaderSize(os.Stdin, 128*1024)
	writer := bufio.NewWriterSize(realStdout, 128*1024)

	if err := run(ctx, stop, buffer, logger, reader, writer); err != nil {
		if isExpectedShutdownErr(err) {
			slog.Info("server shut down gracefully", "error", err)
			if flushErr := writer.Flush(); flushErr != nil {
				slog.Debug("final flush error", "error", flushErr)
			}
			exitFunc(0)
			return
		}
		slog.Error("server fatal error", "error", err)
		exitFunc(1)
	}
	if flushErr := writer.Flush(); flushErr != nil {
		slog.Debug("final flush error", "error", flushErr)
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

func run(ctx context.Context, cancel context.CancelFunc, buffer *system.LogBuffer, logger *slog.Logger, reader io.Reader, writer io.Writer) error {
	eng := engine.NewEngine()

	sequentialthinking.Register(eng)
	system.Register(buffer)

	mcpServer := server.NewMCPServer(config.Name, Version, logger)

	for _, t := range registry.Global.List() {
		t.Register(mcpServer.MCPServer())
	}

	mcpServer.MCPServer().AddResource(&mcp.Resource{
		Name:        "Logs",
		URI:         "sequential-thinking://logs",
		Description: "Server logs",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      request.Params.URI,
					Text:     buffer.String(),
					MIMEType: "text/plain",
				},
			},
		}, nil
	})

	errChan := make(chan error, 1)
	go func(threadCtx context.Context) {
		eofReader := &eofDetector{
			r:      reader,
			cancel: cancel,
		}
		autoWriter := &autoFlusher{w: writer}
		if err := mcpServer.Serve(threadCtx, util.NopWriteCloser{Writer: autoWriter}, util.NopReadCloser{Reader: eofReader}); err != nil {
			select {
			case errChan <- err:
			case <-threadCtx.Done():
			}
		}
	}(ctx)

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

// eofDetector safely monitors for EOF on Read calls to trigger shutdown.
type eofDetector struct {
	r      io.Reader
	cancel context.CancelFunc
}

// Read monitors for EOFs on standard input, canceling the context if found.
func (e *eofDetector) Read(p []byte) (n int, err error) {
	n, err = e.r.Read(p)
	if errors.Is(err, io.EOF) {
		slog.Warn("orchestrator pipe closed (EOF); self-terminating")
		e.cancel()
	}
	return n, err
}

// flusher defines the standard Flush behavior needed for robust writes.
type flusher interface {
	Flush() error
}

// autoFlusher automatically drives underlying buffer flushes on write completion.
type autoFlusher struct {
	w io.Writer
}

// Write handles passing buffered output and flushing only on message boundaries.
func (a *autoFlusher) Write(p []byte) (n int, err error) {
	n, err = a.w.Write(p)
	// Flush only on newline-terminated writes (JSON-RPC message boundary).
	// This preserves the 128KB buffer's coalescing behavior for multi-chunk SDK writes.
	if len(p) > 0 && p[len(p)-1] == '\n' {
		if f, ok := a.w.(flusher); ok {
			if flushErr := f.Flush(); flushErr != nil {
				slog.Debug("auto-flush error", "error", flushErr)
			}
		}
	}
	return n, err
}

