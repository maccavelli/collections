package decision

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
)

func TestCaptureDecisionTool_Handle(t *testing.T) {
	eng := engine.NewEngine(".")
	tool := &CaptureDecisionTool{
		Engine: eng,
	}

	ctx := context.Background()
	input := DecisionInput{
		Decision:     "Select PostgreSQL as the primary database.",
		Alternatives: "MongoDB, DynamoDB",
	}

	// Test Handle
	_, resp, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Errorf("Handle failed: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}

	// Verify Name
	if tool.Name() != "capture_decision_logic" {
		t.Errorf("expected capture_decision_logic, got %s", tool.Name())
	}
}
