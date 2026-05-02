package handler

import (
	"os"
	"path/filepath"
	"testing"

	"mcp-server-magictools/internal/client"
	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/db"
)

func newTestHandler(t *testing.T) (*OrchestratorHandler, *db.Store, *client.WarmRegistry, string) {
	tmpDir, err := os.MkdirTemp("", "handler-test-*")
	if err != nil {
		t.Fatal(err)
	}

	store, err := db.NewStore(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}

	pidsDir := filepath.Join(tmpDir, "pids")
	if err := os.MkdirAll(pidsDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		ManagedServers: []config.ServerConfig{
			{Name: "test-server", Command: "true"},
		},
		SynthesisBiasVector:  0.4,
		SynthesisBiasSynergy: 0.3,
		SynthesisBiasRole:    0.3,
		ScoreFusionAlpha:     0.5,
	}
	reg := client.NewWarmRegistry(pidsDir, store, cfg)
	h := NewHandler(store, reg, cfg)

	return h, store, reg, tmpDir
}
