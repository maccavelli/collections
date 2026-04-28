package dependency

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDependencyTool(t *testing.T) {
	tool := &Tool{}
	if tool.Name() != "go_dependency_impact" {
		t.Errorf("expected go_dependency_impact, got %s", tool.Name())
	}

	// Create temp module
	tmp, _ := os.MkdirTemp("", "dep-test")
	defer os.RemoveAll(tmp)
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module dep-test\n\ngo 1.21\n"), 0644)

	// Test Handle
	input := ImpactInput{
		UniversalPipelineInput: models.UniversalPipelineInput{Target: tmp},
	}
	req := &mcp.CallToolRequest{}

	res, _, err := tool.Handle(context.Background(), req, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if res.IsError {
		t.Errorf("Handle result is error: %v", res.Content)
	}
}

func TestRegister(t *testing.T) {
	e := &engine.Engine{}
	Register(e)
}
