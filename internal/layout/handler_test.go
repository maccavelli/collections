package layout

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
	input := AlignmentInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target:  "mcp-server-go-refactor/internal/layout",
			Context: "AlignmentResult",
		},
	}

	res, _, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed unexpectedly: %v", err)
	}
	if res.IsError {
		t.Fatal("expected success")
	}

	if tool.Name() != "go_struct_alignment_optimizer" {
		t.Errorf("expected go_struct_alignment_optimizer, got %s", tool.Name())
	}
}
