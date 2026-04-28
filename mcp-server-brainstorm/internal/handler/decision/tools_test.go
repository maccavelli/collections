package decision

import (
	"context"
	"testing"

	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
)

func TestCaptureDecisionTool_Handle(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(".", db)
	mgr := state.NewManager(".")
	tool := &CaptureDecisionTool{
		Engine:  eng,
		Manager: mgr,
	}

	ctx := context.Background()
	input := DecisionInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Context: "Select PostgreSQL as the primary database. Alternatives: MongoDB, DynamoDB",
		},
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

// TestRegister removed because tools are now registered via macro tools.
