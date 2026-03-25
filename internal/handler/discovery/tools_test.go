package discovery

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magicskills/internal/engine"
)

func TestListTool_Handle(t *testing.T) {
	eng := engine.NewEngine()
	tool := &ListTool{Engine: eng}

	ctx := context.Background()
	_, _, err := tool.Handle(ctx, &mcp.CallToolRequest{}, struct{}{})
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if tool.Name() != "magicskills_list" {
		t.Errorf("expected magicskills_list, got %s", tool.Name())
	}
}

func TestMatchTool_Handle(t *testing.T) {
	eng := engine.NewEngine()
	tool := &MatchTool{Engine: eng}

	ctx := context.Background()
	input := MatchInput{Intent: "refactor go"}
	_, _, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if tool.Name() != "magicskills_match" {
		t.Errorf("expected magicskills_match, got %s", tool.Name())
	}
}
