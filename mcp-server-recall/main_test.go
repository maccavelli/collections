package main

import (
	"context"
	"os"
	"testing"
	"time"

	"mcp-server-recall/internal/config"
)

func TestRun(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "main-test-*")
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Name:    "test-srv",
		Version: "v1.0.0",
		DBPath:  tmpDir,
	}

	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)

	go func() {
		errChan <- run(ctx, cfg)
	}()

	// Let it start
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("run failed: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("run did not stop gracefully")
	}
}
