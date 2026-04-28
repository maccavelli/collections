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
	defer store.Close()
	defer scn.Watcher.Close()

	if eng == nil || scn == nil || h == nil {
		t.Fatal("expected non-nil server components")
	}
}

func TestInitSubsystems(t *testing.T) {
	t.Setenv("MAGIC_SKILLS_DATA_DIR", t.TempDir())
	lb := &handler.LogBuffer{}
	store, eng, scn, _, _ := initServer(lb, nil)
	defer store.Close()
	defer scn.Watcher.Close()

	initSubsystems(t.Context(), eng, scn, lb, nil)
}
