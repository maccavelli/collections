package discovery

import (
	"context"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/state"
)

func TestDiscoverProjectTool_Handle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "brainstorm-discovery-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := state.NewManager(tmpDir)
	eng := engine.NewEngine(tmpDir)
	tool := &DiscoverProjectTool{
		Manager: mgr,
		Engine:  eng,
	}

	ctx := context.Background()
	input := DiscoverInput{
		Path: tmpDir,
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
