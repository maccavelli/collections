package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"

	"mcp-server-recall/internal/config"
	"mcp-server-recall/internal/memory"
)

func TestWriteSnapshot(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set dbPath in viper so config.New picks it up
	viper.Set("dbPath", tmpDir)
	cfg := config.New("1.0.0-test")

	store, err := memory.NewMemoryStore(context.Background(), tmpDir, "", 1000, config.BatchConfig{MaxBatchSize: 100})
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer store.Close()

	logMsg := "test log line"
	logStream := func() string {
		return logMsg
	}

	// Initial snapshot
	WriteSnapshot(cfg, store, logStream)

	ringPath := filepath.Join(tmpDir, "telemetry.ring")
	if _, err := os.Stat(ringPath); os.IsNotExist(err) {
		t.Errorf("telemetry.ring was not created")
	}

	data, err := os.ReadFile(ringPath)
	if err != nil {
		t.Fatalf("failed to read telemetry.ring: %v", err)
	}

	if len(data) == 0 {
		t.Errorf("telemetry.ring is empty")
	}
}

func TestStartTelemetryLoop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-telemetry-loop-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set dbPath in viper so config.New picks it up
	viper.Set("dbPath", tmpDir)
	cfg := config.New("1.0.0-test")

	store, err := memory.NewMemoryStore(context.Background(), tmpDir, "", 1000, config.BatchConfig{MaxBatchSize: 100})
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer store.Close()

	logStream := func() string { return "loop test" }

	// Start loop (it runs in background)
	StartTelemetryLoop(cfg, store, logStream)

	// Wait a bit to ensure it doesn't panic immediately
	time.Sleep(100 * time.Millisecond)

	// We can't easily wait for the ticker (10s) without refactoring StartTelemetryLoop 
	// to take a duration or a ticker, but we've at least verified it starts.
}
