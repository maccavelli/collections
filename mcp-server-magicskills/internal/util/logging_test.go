package util

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSetupStandardLogging(t *testing.T) {
	var buf bytes.Buffer
	cleanup := SetupStandardLogging("test_server", &buf)
	slog.Info("test message")
	time.Sleep(100 * time.Millisecond)
	cleanup()
	if buf.String() == "" {
		t.Errorf("expected logs in buffer, got empty")
	}
}

func TestOpenHardenedLogFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	f := OpenHardenedLogFile(path)
	f.Close()

	// Write 51MB of data with newlines so truncation can snap to a boundary
	data := make([]byte, 51*1024*1024)
	for i := range data {
		data[i] = 'a'
	}
	for i := 1024; i < len(data); i += 1024 {
		data[i] = '\n'
	}
	os.WriteFile(path, data, 0644)

	f2 := OpenHardenedLogFile(path)
	f2.Close()

	info, _ := os.Stat(path)
	// Graceful truncation retains ~5MB tail
	const truncateTarget = 5 * 1024 * 1024
	if info.Size() > int64(truncateTarget+1024) {
		t.Errorf("expected file to be truncated to ~%d bytes, got size %d", truncateTarget, info.Size())
	}
	if info.Size() == 0 {
		t.Error("expected file to retain tail data, got 0 bytes")
	}
}

func TestAsyncWriter(t *testing.T) {
	var buf bytes.Buffer
	aw := NewAsyncWriter(&buf, 10)
	aw.Write([]byte("test log\n"))
	time.Sleep(100 * time.Millisecond)
	if err := aw.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
	aw.Write([]byte("dropped log after close\n"))

	if output := buf.String(); output != "test log\n" {
		t.Errorf("expected 'test log\\n', got %q", output)
	}
}
