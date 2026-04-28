// Package main implements the MCP filesystem server, providing sandboxed
// file-system operations over the Model Context Protocol stdio transport.
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
	"mcp-server-filesystem/internal/config"
	"mcp-server-filesystem/internal/handler/filesystem"
	"mcp-server-filesystem/internal/handler/system"
	"mcp-server-filesystem/internal/pathutil"
	"mcp-server-filesystem/internal/registry"
	"mcp-server-filesystem/internal/server"
	"mcp-server-filesystem/internal/util"
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

	// Allowed directories from arguments.
	dirs := flag.Args()
	if len(dirs) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: mcp-server-filesystem [allowed-directory] [additional-directories...]")
		fmt.Fprintln(os.Stderr, "Note: Allowed directories can also be provided via MCP roots protocol.")
	}

	// Redirect os.Stdout to stderr so only MCP JSON goes to real stdout.
	realStdout := os.Stdout
	os.Stdout = os.Stderr

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	buffer := &system.LogBuffer{}
	cleanupLogs := util.SetupStandardLogging(ctx, "filesystem", buffer)
	defer cleanupLogs()

	logger := slog.Default()
	slog.Info("[BACKPLANE] SPAWN "+config.Name, "version", Version, "allowed_dirs", dirs)

	reader := bufio.NewReaderSize(os.Stdin, 128*1024)
	writer := bufio.NewWriterSize(realStdout, 128*1024)

	if err := run(ctx, stop, dirs, buffer, logger, reader, writer); err != nil {
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

// isExpectedShutdownErr reports whether err represents a benign shutdown
// condition such as EOF, broken pipe, or a closed connection.
func isExpectedShutdownErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, phrase := range []string{
		"eof", "broken pipe", "connection reset", "use of closed",
		"file already closed", "bad file descriptor",
		"client is closing", "connection closed",
	} {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}

func run(ctx context.Context, cancel context.CancelFunc, dirs []string, buffer *system.LogBuffer, logger *slog.Logger, reader io.Reader, writer io.Writer) error {
	pm := pathutil.NewManager(dirs)

	// Filter to accessible directories.
	accessible := filterAccessible(pm.Allowed())
	if len(accessible) == 0 && len(dirs) > 0 {
		slog.Warn("none of the specified directories are accessible, waiting for MCP roots")
	}

	filesystem.Register(pm)
	system.Register(buffer)

	mcpServer := server.NewMCPServer(config.Platform, Version, logger)

	for _, t := range registry.Global.List() {
		t.Register(mcpServer.MCPServer())
	}

	// Log resource.
	mcpServer.MCPServer().AddResource(&mcp.Resource{
		Name:        "Logs",
		URI:         "filesystem://logs",
		Description: "[DIRECTIVE: Local FS Manipulation] Server logs Keywords: file, os, disk, local, path, system",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      req.Params.URI,
					Text:     buffer.String(),
					MIMEType: "text/plain",
				},
			},
		}, nil
	})

	errChan := make(chan error, 1)
	go func() {
		eofReader := &eofDetector{r: reader, cancel: cancel}
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

// filterAccessible returns only the directories that exist and are accessible.
func filterAccessible(dirs []string) []string {
	var result []string
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			slog.Warn("cannot access directory, skipping", "dir", dir, "error", err)
			continue
		}
		if !info.IsDir() {
			slog.Warn("path is not a directory, skipping", "dir", dir)
			continue
		}
		result = append(result, dir)
	}
	return result
}

// eofDetector wraps an io.Reader and triggers context cancellation when the
// underlying reader returns io.EOF, enabling graceful orchestrator shutdown.
type eofDetector struct {
	r      io.Reader
	cancel context.CancelFunc
}

// Read implements io.Reader. It forwards reads to the underlying reader and
// invokes the cancel function when io.EOF is encountered.
func (e *eofDetector) Read(p []byte) (n int, err error) {
	n, err = e.r.Read(p)
	if err == io.EOF {
		slog.Warn("orchestrator pipe closed (EOF); self-terminating")
		e.cancel()
	}
	return n, err
}

// flusher is satisfied by writers that support explicit buffer flushing (e.g. bufio.Writer).
type flusher interface {
	Flush() error
}

// autoFlusher wraps an io.Writer and automatically flushes on newline-terminated
// writes, aligning flush boundaries with JSON-RPC message delimiters.
type autoFlusher struct {
	w io.Writer
}

// Write implements io.Writer. It delegates to the underlying writer and
// triggers an auto-flush when the payload ends with a newline byte.
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
