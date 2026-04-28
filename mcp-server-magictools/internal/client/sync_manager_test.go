package client

import (
	"context"
	"os"
	"testing"

	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/db"
)

func TestSyncEcosystem_Empty(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "magictools-sync-test-*")
	defer os.RemoveAll(tempDir)

	store, _ := db.NewStore(tempDir)
	cfg := &config.Config{}
	m := NewWarmRegistry(tempDir, store, cfg)

	// Should not panic, just return nil error if no servers
	_, err := m.SyncEcosystem(context.Background())
	if err != nil {
		t.Errorf("SyncEcosystem failed: %v", err)
	}
}

func TestSyncServer_NotFound(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "magictools-sync-test-*")
	defer os.RemoveAll(tempDir)

	store, _ := db.NewStore(tempDir)
	cfg := &config.Config{}
	m := NewWarmRegistry(tempDir, store, cfg)

	err := m.SyncServer(context.Background(), "unknown")
	if err == nil {
		t.Error("expected error for unknown server")
	}
}
