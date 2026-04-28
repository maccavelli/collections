package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
)

func TestDiscoverProjectTool_Handle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "brainstorm-discovery-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Add a sentinel to prevent walking up to system /tmp
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)

	mgr := state.NewManager(tmpDir)
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(tmpDir, db)
	tool := &DiscoverProjectTool{
		Manager: mgr,
		Engine:  eng,
	}

	ctx := context.Background()
	input := DiscoverInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target: tmpDir,
		},
	}

	// Test Handle
	_, resp, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Errorf("Handle failed: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}

	// Verify Name
	if tool.Name() != "discover_project" {
		t.Errorf("expected discover_project, got %s", tool.Name())
	}
}

func TestRegister(t *testing.T) {
	mgr := state.NewManager(".")
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(".", db)

	Register(mgr, eng)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, &mcp.ServerOptions{})
	tool := &DiscoverProjectTool{Manager: mgr, Engine: eng}
	tool.Register(&util.MockSessionProvider{Srv: srv})
}
