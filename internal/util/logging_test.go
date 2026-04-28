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
	if len(buf.String()) == 0 {
		t.Errorf("expected logs in buffer, got empty")
	}
}

func TestOpenHardenedLogFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	f := OpenHardenedLogFile(path)
	f.Close()

	content := bytes.Repeat([]byte("a"), 51*1024*1024)
	os.WriteFile(path, content, 0644)

	f2 := OpenHardenedLogFile(path)
	f2.Close()

	info, _ := os.Stat(path)
	if info.Size() != 0 {
		t.Errorf("expected file to be truncated to 0, got size %d", info.Size())
	}
}
