package contextanalysis

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
	input := ContextInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target: "mcp-server-go-refactor/internal/analysis/context",
		},
	}

	res, _, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed unexpectedly: %v", err)
	}
	if res.IsError {
		t.Fatal("expected success")
	}

	if tool.Name() != "go_context_analyzer" {
		t.Errorf("expected go_context_analyzer, got %s", tool.Name())
	}
}
