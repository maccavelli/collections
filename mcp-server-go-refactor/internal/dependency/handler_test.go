package dependency

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
	input := ImpactInput{
		UniversalPipelineInput: models.UniversalPipelineInput{Target: "mcp-server-go-refactor/internal/dependency"},
	}

	res, _, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed unexpectedly: %v", err)
	}
	if res.IsError {
		t.Fatal("expected success")
	}

	if tool.Name() != "go_dependency_impact" {
		t.Errorf("expected go_dependency_impact, got %s", tool.Name())
	}
}
