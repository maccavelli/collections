package util

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// AsyncWriter provides non-blocking, buffered writes to an underlying io.Writer (usually Stderr).
// This is critical for bastion environments where blocking on SSH stderr can stall the main process.
type AsyncWriter struct {
	writer      io.Writer
	ch          chan []byte
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	dropped     int64
	maxDuration time.Duration
	closed      atomic.Bool
}

// NewAsyncWriter creates a new AsyncWriter with the given channel capacity.
func NewAsyncWriter(ctx context.Context, w io.Writer, capacity int) *AsyncWriter {
	childCtx, cancel := context.WithCancel(ctx)
	aw := &AsyncWriter{
		writer:      w,
		ch:          make(chan []byte, capacity),
		ctx:         childCtx,
		cancel:      cancel,
		maxDuration: 100 * time.Millisecond,
	}
	aw.wg.Add(1)
	go aw.worker(childCtx)
	return aw
}

func (aw *AsyncWriter) worker(ctx context.Context) {
	defer aw.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case p, ok := <-aw.ch:
			if !ok {
				return
			}
			if _, err := aw.writer.Write(p); err != nil {
				atomic.AddInt64(&aw.dropped, 1)
			}
		}
	}
}

// Write buffers data to the underlying channel or drops it if max duration is reached.
func (aw *AsyncWriter) Write(p []byte) (n int, err error) {
	if aw.closed.Load() {
		return len(p), nil
	}
	// Copy buffer to avoid race conditions with Caller-managed buffers
	data := make([]byte, len(p))
	copy(data, p)

	timer := time.NewTimer(aw.maxDuration)
	defer timer.Stop()

	select {
	case <-aw.ctx.Done():
		return 0, aw.ctx.Err()
	case aw.ch <- data:
		return len(p), nil
	case <-timer.C:
		atomic.AddInt64(&aw.dropped, 1)
		return len(p), nil // Dropping logs is better than blocking the main task on a bastion
	}
}

// Close signals the worker to finish, waits for completion, and cancels the background context.
func (aw *AsyncWriter) Close() error {
	if aw.closed.Swap(true) {
		return nil // idempotent: already closed
	}
	close(aw.ch)  // Signal worker via channel close
	aw.wg.Wait()  // Wait for worker to drain all pending messages
	aw.cancel()   // Then cancel context (cleanup)
	return nil
}

// OpenHardenedLogFile opens a file with a 10MB safety cap for Bastion environments.
// If the file exceeds 10MB, it is truncated to 0.
func OpenHardenedLogFile(path string) *os.File {
	const maxLogSize = 50 * 1024 * 1024 // 50MB
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
		if err := os.Truncate(path, 0); err != nil {
			slog.Error("failed to truncate oversized log file", "path", path, "error", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return os.Stderr
	}
	return f
}

// LogFanout writes to an in-memory buffer first (fast path), then to the async file writer.
// This avoids io.MultiWriter's serialization penalty where both writers hold locks.
type LogFanout struct {
	Buffer io.Writer // in-memory LogBuffer, always fast
	Async  *AsyncWriter // file writer, may drop under pressure
}

// Write sends data to the buffer first, then forwards to the async writer.
func (f *LogFanout) Write(p []byte) (int, error) {
	if _, err := f.Buffer.Write(p); err != nil {
		// We use slog.Error here, but careful not to loop if this logger itself is failing.
		// However, f.Buffer is usually an in-memory buffer.
		slog.Error("log fanout: buffer write failed", "error", err)
	}
	return f.Async.Write(p)
}

// SetupStandardLogging configures a non-blocking JSON logger for the bastion host.
// It ensures that No MCP server logs to Stdout.
func SetupStandardLogging(ctx context.Context, serverName string, buffer io.Writer) func() {
	// 🛡️ Stderr Isolation: Redirect logs to a dedicated file to keep stderr clean for JSON-RPC
	var writers []io.Writer

	logDir := filepath.Join(os.TempDir(), "mcp-server-"+serverName)
	if err := os.MkdirAll(logDir, 0700); err != nil {
		slog.Error("failed to create secure log directory", "dir", logDir, "error", err)
		logDir = os.TempDir()
	}
	localLogPath := filepath.Join(logDir, "mcp-subserver-"+serverName+".log")

	localLogFile := OpenHardenedLogFile(localLogPath)

	// On heavy load, we drop logs rather than stall the server connection.
	localAw := NewAsyncWriter(ctx, localLogFile, 1024)
	writers = append(writers, localAw)

	var globalLogFile *os.File
	var globalAw *AsyncWriter
	if envPath := os.Getenv("MCP_LOG_FILE"); envPath != "" {
		globalLogFile = OpenHardenedLogFile(envPath)
		globalAw = NewAsyncWriter(ctx, globalLogFile, 1024)
		writers = append(writers, globalAw)
	}

	if buffer != nil {
		writers = append(writers, buffer)
	}

	sw := io.MultiWriter(writers...)

	lvl := new(slog.LevelVar)
	lvl.Set(slog.LevelInfo) // Baseline default standalone protection

	if val := os.Getenv("ORCHESTRATOR_LOG_LEVEL"); val != "" {
		switch strings.ToUpper(val) {
		case "DEBUG":
			lvl.Set(slog.LevelDebug)
		case "INFO":
			lvl.Set(slog.LevelInfo)
		case "WARN", "WARNING":
			lvl.Set(slog.LevelWarn)
		case "ERROR", "CRITICAL":
			lvl.Set(slog.LevelError)
		}
	}

	format := os.Getenv("ORCHESTRATOR_LOG_FORMAT")
	if format == "" {
		format = "json"
	}

	var handler slog.Handler
	if strings.ToLower(format) == "text" {
		handler = slog.NewTextHandler(sw, &slog.HandlerOptions{Level: lvl})
	} else {
		handler = slog.NewJSONHandler(sw, &slog.HandlerOptions{Level: lvl})
	}
	slog.SetDefault(slog.New(handler).With("server", serverName))

	return func() {
		if err := localAw.Close(); err != nil {
			slog.Error("failed to close async log writer", "error", err)
		}
		if localLogFile != nil && localLogFile != os.Stderr {
			if err := localLogFile.Close(); err != nil {
				slog.Error("failed to close physical log file", "path", localLogPath, "error", err)
			}
		}
		if globalAw != nil {
			if err := globalAw.Close(); err != nil {
				slog.Error("failed to close global async log writer", "error", err)
			}
		}
		if globalLogFile != nil && globalLogFile != os.Stderr {
			if err := globalLogFile.Close(); err != nil {
				slog.Error("failed to close global physical log file", "error", err)
			}
		}
	}
}
