package util

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAsyncWriter(t *testing.T) {
	var buf bytes.Buffer
	aw := NewAsyncWriter(&buf, 10)

	aw.Write([]byte("test log\n"))

	// Allow time for worker to process
	time.Sleep(min(200*time.Millisecond, aw.maxDuration+50*time.Millisecond))

	if err := aw.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	aw.Write([]byte("dropped log after close\n")) // Should be dropped

	output := buf.String()
	if output != "test log\n" {
		t.Errorf("expected 'test log\\n', got %q", output)
	}
}

func TestSetupStandardLogging(t *testing.T) {
	var buf bytes.Buffer
	cleanup := SetupStandardLogging("test_server", &buf)

	slog.Info("test message")
	time.Sleep(100 * time.Millisecond) // wait for async write

	cleanup()

	output := buf.String()
	if len(output) == 0 {
		t.Errorf("expected logs in buffer, got empty")
	}
}

func TestOpenHardenedLogFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	f := OpenHardenedLogFile(path)
	if f == nil {
		t.Fatal("expected file, got nil")
	}
	f.Close()

	// Create oversized file to trigger truncation
	content := bytes.Repeat([]byte("a"), 51*1024*1024)
	os.WriteFile(path, content, 0644)

	f2 := OpenHardenedLogFile(path)
	f2.Close()

	info, _ := os.Stat(path)
	if info.Size() != 0 {
		t.Errorf("expected file to be truncated to 0, got size %d", info.Size())
	}

	defer os.Remove(path)
}
