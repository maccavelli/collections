package graph

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_Handle(t *testing.T) {
	tool := &Tool{}
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	input := CyclerInput{
		Pkg: "mcp-server-go-refactor/internal/graph",
	}

	res, _, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed unexpectedly: %v", err)
	}
	if res.IsError {
		t.Fatal("expected success")
	}

	if tool.Name() != "go_package_cycler" {
		t.Errorf("expected go_package_cycler, got %s", tool.Name())
	}
}
