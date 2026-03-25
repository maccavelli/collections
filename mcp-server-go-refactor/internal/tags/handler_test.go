package tags

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_Handle(t *testing.T) {
	tool := &Tool{}
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	input := TagInput{
		Pkg:        "mcp-server-go-refactor/internal/tags",
		StructName: "TagResult",
		TargetTag:  "json",
		CaseFormat: "snake",
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
