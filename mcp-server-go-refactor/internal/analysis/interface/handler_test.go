package interfaceanalysis

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
	input := DiscoveryInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target: "mcp-server-go-refactor/internal/analysis/interface",
		},
	}

	res, _, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed unexpectedly: %v", err)
	}
	if res.IsError {
		t.Fatal("expected success")
	}

	if tool.Name() != "go_interface_discovery" {
		t.Errorf("expected go_interface_discovery, got %s", tool.Name())
	}
}
