package pruner

import (
	"context"
	"testing"

	"mcp-server-go-refactor/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_Handle(t *testing.T) {
	tool := &Tool{}
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	input := PruneInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target: "mcp-server-go-refactor/internal/pruner",
		},
	}

	res, _, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if res.IsError {
		t.Fatal("expected success")
	}

	if tool.Name() != "go_dead_code_pruner" {
		t.Errorf("expected go_dead_code_pruner, got %s", tool.Name())
	}
}
