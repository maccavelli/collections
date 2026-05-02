package coverage

import (
	"context"
	"os"
	"testing"

	"mcp-server-go-refactor/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_Handle(t *testing.T) {
	if os.Getenv("GO_TEST_JSON") == "1" {
		t.Skip("Skipping recursive trace")
	}
	os.Setenv("GO_TEST_JSON", "1")
	defer os.Unsetenv("GO_TEST_JSON")

	tool := &Tool{}
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	input := CoverageInput{
		UniversalPipelineInput: models.UniversalPipelineInput{Target: "mcp-server-go-refactor/internal/coverage"},
	}

	res, _, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if res.IsError {
		t.Fatal("expected success")
	}

	if tool.Name() != "go_test_coverage_tracer" {
		t.Errorf("expected go_test_coverage_tracer, got %s", tool.Name())
	}
}
