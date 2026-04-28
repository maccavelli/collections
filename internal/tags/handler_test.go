package tags

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
	input := TagInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target:  "mcp-server-go-refactor/internal/tags",
			Context: "TagResult",
			Flags:   map[string]any{"targetTag": "json", "caseFormat": "snake"},
		},
	}

	res, _, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed unexpectedly: %v", err)
	}
	if res.IsError {
		t.Fatal("expected success")
	}

	if tool.Name() != "go_tag_manager" {
		t.Errorf("expected go_tag_manager, got %s", tool.Name())
	}
}
