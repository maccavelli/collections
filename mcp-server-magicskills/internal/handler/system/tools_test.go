package system

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/scanner"
)

func TestAddRootTool_Handle(t *testing.T) {
	eng := engine.NewEngine()
	scn, _ := scanner.NewScanner([]string{"."})
	tool := &AddRootTool{Engine: eng, Scanner: scn}

	ctx := context.Background()
	input := AddRootInput{Path: "."}

	// Test Handle
	_, resp, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}

	// Verify Name
	if tool.Name() != "magicskills_add_root" {
		t.Errorf("expected magicskills_add_root, got %s", tool.Name())
	}
}
