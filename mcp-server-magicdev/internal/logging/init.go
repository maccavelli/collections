// Package logging provides functionality for the logging subsystem.
package logging

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	// LogFileName is the output log file written to the OS cache directory.
	LogFileName = "magicdev-output.log"

	// MaxLogFileSize is the threshold above which the log file is truncated on startup.
	MaxLogFileSize = 10 * 1024 * 1024 // 10MB

	// TruncateTarget is the size to retain after truncation.
	TruncateTarget = 5 * 1024 * 1024 // 5MB
)

var (
	globalLevel *slog.LevelVar
	logFile     *os.File
	initMu      sync.Mutex

	// sanitizeRegex matches common secret patterns in log output.
	sanitizeRegex = regexp.MustCompile(`(?i)(token_|sk_|key_|secret_|bearer |authorization: )[a-zA-Z0-9_./-]+`)
)

// SanitizingWriter wraps an io.Writer and redacts secrets before writing.
type SanitizingWriter struct {
	inner io.Writer
}

// Write redacts secrets from p before forwarding to the inner writer.
func (sw *SanitizingWriter) Write(p []byte) (int, error) {
	redacted := sanitizeRegex.ReplaceAll(p, []byte("${1}[REDACTED]"))
	_, err := sw.inner.Write(redacted)
	// Return original length for slog handler compatibility.
	return len(p), err
}

// ParseLevel converts a string log level to slog.Level.
// Unrecognized values default to INFO.
func ParseLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Reconfigure (re)initializes the global logger with a persistent file writer.
// It opens magicdev-output.log in the OS cache directory and creates a JSON
// handler writing to stderr + GlobalBuffer + log file with secret redaction.
// Returns the absolute log file path.
func Reconfigure(logLevel string) (string, error) {
	initMu.Lock()
	defer initMu.Unlock()

	// Resolve OS-idempotent cache directory.
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve cache dir: %w", err)
	}
	logDir := filepath.Join(cacheDir, "mcp-server-magicdev")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return "", fmt.Errorf("create log dir: %w", err)
	}
	logPath := filepath.Join(logDir, LogFileName)

	// Truncate if oversized to prevent unbounded growth.
	truncateIfOversized(logPath)

	// Close previous file handle if Reconfigure is called more than once.
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}

	// Open log file in append mode (crash-safe).
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("open log file: %w", err)
	}
	logFile = f

	// Create or update the dynamic level variable.
	if globalLevel == nil {
		globalLevel = new(slog.LevelVar)
	}
	globalLevel.Set(ParseLevel(logLevel))

	// Build the sanitized multi-writer chain:
	// SanitizingWriter → MultiWriter → [stderr, GlobalBuffer, logFile]
	mw := io.MultiWriter(os.Stderr, GlobalBuffer, logFile)
	sanitized := &SanitizingWriter{inner: mw}

	handler := slog.NewJSONHandler(sanitized, &slog.HandlerOptions{
		Level: globalLevel,
	})
	slog.SetDefault(slog.New(handler))

	return logPath, nil
}

// SetLevel atomically updates the active log level.
// Safe to call from any goroutine (e.g., fsnotify hot-reload callback).
func SetLevel(logLevel string) {
	if globalLevel != nil {
		globalLevel.Set(ParseLevel(logLevel))
	}
}

// Level returns the current slog.Level, or INFO if not yet initialized.
func Level() slog.Level {
	if globalLevel != nil {
		return globalLevel.Level()
	}
	return slog.LevelInfo
}

// CloseLogFile closes the underlying log file handle for clean shutdown.
func CloseLogFile() {
	initMu.Lock()
	defer initMu.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

// truncateIfOversized checks if the log file exceeds MaxLogFileSize
// and if so, retains only the last TruncateTarget bytes, aligning to
// a newline boundary to avoid partial JSON lines.
func truncateIfOversized(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= MaxLogFileSize {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	// Keep the tail.
	start := len(data) - TruncateTarget
	if start < 0 {
		return
	}

	// Advance past the first newline to avoid a partial JSON line.
	if idx := bytes.IndexByte(data[start:], '\n'); idx >= 0 {
		start += idx + 1
	}

	_ = os.WriteFile(path, data[start:], 0600)
}
