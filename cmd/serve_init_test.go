package cmd

import (
	"mcp-server-magicskills/internal/handler"
	"testing"
)

func TestInitServer(t *testing.T) {
	t.Setenv("MAGIC_SKILLS_DATA_DIR", t.TempDir())
	t.Setenv("MCP_SKILLS_ROOT", t.TempDir())

	lb := &handler.LogBuffer{}
	store, eng, scn, h, err := initServer(lb, []string{t.TempDir()})
	if err != nil {
		t.Fatalf("initServer failed: %v", err)
	}
	defer func() { _ = store.Close() }()       // nolint:errcheck // ignore close error
	defer func() { _ = scn.Watcher.Close() }() // nolint:errcheck // ignore close error

	if eng == nil || scn == nil || h == nil {
		t.Fatal("expected non-nil server components")
	}
}

func TestInitSubsystems(t *testing.T) {
	t.Setenv("MAGIC_SKILLS_DATA_DIR", t.TempDir())
	lb := &handler.LogBuffer{}
	store, eng, scn, _, _ := initServer(lb, nil) // nolint:errcheck // ignore initServer error
	defer func() { _ = store.Close() }()         // nolint:errcheck // ignore close error
	defer func() { _ = scn.Watcher.Close() }()   // nolint:errcheck // ignore close error

	initSubsystems(t.Context(), eng, scn, lb)
}
