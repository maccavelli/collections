package client

import (
	"context"
	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/db"
	"os"
	"testing"
	"time"
)

func TestRegistryExhaustive(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "client-test")
	defer os.RemoveAll(tmpDir)
	store, _ := db.NewStore(tmpDir)
	cfg := &config.Config{
		ManagedServers: []config.ServerConfig{{Name: "fake", Command: "foo"}},
	}

	reg := NewWarmRegistry(tmpDir, store, cfg)
	ctx := context.Background()

	// Error paths for large functions
	_ = reg.Connect(ctx, "fake", "bad-command-xyz-123", []string{}, nil, "hash")
	reg.Boot(ctx, []config.ServerConfig{})
	reg.AuditGlobalRegistry()
	_, _ = reg.CallProxy(ctx, "fake", "fakeTool", map[string]any{}, 30*time.Second)

	_, _ = reg.SyncEcosystem(ctx)
	_ = reg.SyncServer(ctx, "fake")

	reg.DisconnectAll()
	reg.GetServer("fake")
}
