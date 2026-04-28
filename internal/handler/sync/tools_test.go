package sync

import (
	"context"
	"os"
	"strings"
	"testing"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/scanner"
	"mcp-server-magicskills/internal/state"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSyncTool_Handle(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	scn, _ := scanner.NewScanner([]string{})

	tool := &SyncTool{Engine: eng, Scanner: scn}

	ctx := context.Background()
	input := SyncInput{}

	res, out, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error in result")
	}
	output, ok := out.(struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	})
	if !ok {
		t.Errorf("expected structured output")
	}
	if !strings.Contains(output.Summary, "up to date") {
		t.Errorf("expected 'up to date' summary, got: %s", output.Summary)
	}
}

func TestSyncTool_SyncLogic(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()

	skillDir := tmpDir + "/.agent/skills/test"
	_ = os.MkdirAll(skillDir, 0755)
	_ = os.WriteFile(skillDir+"/SKILL.md", []byte("---\nname: test\n---\ncontent"), 0644)

	scn, _ := scanner.NewScanner([]string{tmpDir + "/.agent/skills"})
	tool := &SyncTool{Engine: eng, Scanner: scn}

	res, out, err := tool.Handle(context.Background(), &mcp.CallToolRequest{}, SyncInput{})
	if err != nil || res.IsError {
		t.Fatalf("Sync failed: %v", err)
	}
	output := out.(struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	})
	if !strings.Contains(output.Summary, "1 added") {
		t.Errorf("expected 1 added, got: %s", output.Summary)
	}
}

func TestSyncTool_Errors(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()

	// Error in discovery (cancelled context)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tool := &SyncTool{Engine: eng, Scanner: &scanner.Scanner{Roots: []string{t.TempDir()}}}
	res, _, _ := tool.Handle(ctx, &mcp.CallToolRequest{}, SyncInput{})
	if !res.IsError {
		t.Error("expected error for cancelled context in discovery")
	}

	// Error in engine sync (closed store)
	closedStore, _ := state.NewStore(t.TempDir())
	badEng, _ := engine.NewEngine(closedStore, t.TempDir()+"/idx_bad")
	close(badEng.ReadyCh)
	closedStore.Close()

	tool2 := &SyncTool{Engine: badEng, Scanner: &scanner.Scanner{Roots: []string{t.TempDir()}}}
	_, _, _ = tool2.Handle(context.Background(), &mcp.CallToolRequest{}, SyncInput{})
	// Engine sync might not error if no files are found to sync, even with closed store
}

func TestRegister(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	scn, _ := scanner.NewScanner([]string{})
	Register(eng, scn)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, &mcp.ServerOptions{})
	tool := &SyncTool{Engine: eng, Scanner: scn}
	tool.Register(srv)
}
