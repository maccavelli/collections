package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"  Debug ", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"info", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"INVALID", slog.LevelInfo},
		{"trace", slog.LevelInfo},
	}

	for _, tc := range tests {
		got := ParseLevel(tc.input)
		if got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestReconfigure(t *testing.T) {
	// Use a temp dir to avoid polluting the real cache.
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	logPath, err := Reconfigure("DEBUG")
	if err != nil {
		t.Fatalf("Reconfigure failed: %v", err)
	}

	// Verify log file was created.
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatalf("Log file was not created at %s", logPath)
	}

	// Verify the file path ends correctly.
	if !strings.HasSuffix(logPath, LogFileName) {
		t.Errorf("Log path %q does not end with %q", logPath, LogFileName)
	}

	// Verify level was applied.
	if Level() != slog.LevelDebug {
		t.Errorf("Level() = %v, want DEBUG", Level())
	}

	// Write a log entry and verify it appears in the file.
	slog.Info("test log entry from TestReconfigure")

	// Read the file back.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "TestReconfigure") {
		t.Error("Log file does not contain the test log entry")
	}

	CloseLogFile()
}

func TestSetLevel(t *testing.T) {
	// Ensure globalLevel is initialized.
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)
	if _, err := Reconfigure("INFO"); err != nil {
		t.Fatalf("Reconfigure failed: %v", err)
	}

	// Start at INFO.
	if Level() != slog.LevelInfo {
		t.Errorf("Initial level = %v, want INFO", Level())
	}

	// Switch to ERROR.
	SetLevel("ERROR")
	if Level() != slog.LevelError {
		t.Errorf("After SetLevel(ERROR), got %v", Level())
	}

	// Switch to DEBUG.
	SetLevel("DEBUG")
	if Level() != slog.LevelDebug {
		t.Errorf("After SetLevel(DEBUG), got %v", Level())
	}

	CloseLogFile()
}

func TestTruncateIfOversized(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create a file larger than MaxLogFileSize (use smaller constants for testing).
	// We'll write 12MB of data and verify truncation.
	bigData := make([]byte, 12*1024*1024)
	for i := range bigData {
		bigData[i] = 'A'
		if i > 0 && i%100 == 0 {
			bigData[i] = '\n'
		}
	}
	if err := os.WriteFile(logPath, bigData, 0600); err != nil {
		t.Fatalf("Failed to write big log file: %v", err)
	}

	// Verify it's oversized.
	info, _ := os.Stat(logPath)
	if info.Size() <= MaxLogFileSize {
		t.Fatalf("Test file should be > %d bytes, got %d", MaxLogFileSize, info.Size())
	}

	// Truncate.
	truncateIfOversized(logPath)

	// Verify it's now <= TruncateTarget + a line boundary buffer.
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("Stat after truncation failed: %v", err)
	}
	if info.Size() > int64(TruncateTarget)+200 {
		t.Errorf("After truncation, file is %d bytes, expected <= %d", info.Size(), TruncateTarget+200)
	}
	if info.Size() < int64(TruncateTarget)-200 {
		t.Errorf("After truncation, file is %d bytes, expected >= %d", info.Size(), TruncateTarget-200)
	}
}

func TestSanitizingWriter(t *testing.T) {
	var buf strings.Builder
	sw := &SanitizingWriter{inner: &buf}

	input := `{"msg":"connecting","token_abc123def":"value","key_secret_xyz":"data"}`
	_, err := sw.Write([]byte(input))
	if err != nil {
		t.Fatalf("SanitizingWriter.Write failed: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "abc123def") {
		t.Error("Secret token was not redacted")
	}
	if strings.Contains(output, "secret_xyz") {
		t.Error("Secret key was not redacted")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("Expected [REDACTED] placeholder in output")
	}
}
