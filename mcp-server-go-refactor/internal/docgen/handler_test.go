package docgen

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
	input := DocInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target: "mcp-server-go-refactor/internal/docgen",
		},
	}

	res, _, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if res.IsError {
		t.Fatal("expected success")
	}

	if tool.Name() != "go_doc_generator" {
		t.Errorf("expected go_doc_generator, got %s", tool.Name())
	}
}
