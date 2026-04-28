//go:build !windows && !android

package telemetry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRingBuffer(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.ring")
	MustInitializeRingBuffer(tmpFile)
	if GlobalRingBuffer == nil {
		t.Fatal("GlobalRingBuffer is nil")
	}
	// Clean up GlobalRingBuffer so we don't leak state into other tests
	defer func() {
		GlobalRingBuffer.file.Close()
		GlobalRingBuffer = nil
	}()

	err := GlobalRingBuffer.WriteGauges(map[string]string{"test": "ok"})
	if err != nil {
		t.Fatal(err)
	}

	GlobalRingBuffer.WriteRecord([]byte(`{"msg":"hello world"}`))

	gauges, logs, err := ReadState(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(gauges) == 0 || len(logs) == 0 {
		t.Fatal("empty reads")
	}
}

func TestReadStateMissingFile(t *testing.T) {
	gauges, logs, err := ReadState("/nonexistent/path/to/ring")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if gauges != nil || logs != nil {
		t.Fatal("expected nil slices for missing file")
	}
}

func TestReadStateUndersizedFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "small.ring")
	if err := os.WriteFile(tmpFile, make([]byte, 100), 0666); err != nil {
		t.Fatal(err)
	}
	gauges, logs, err := ReadState(tmpFile)
	if err != nil {
		t.Fatalf("expected nil error for undersized file, got: %v", err)
	}
	if gauges != nil || logs != nil {
		t.Fatal("expected nil slices for undersized file")
	}
}
