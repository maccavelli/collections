package decision

import (
	"context"
	"os"
	"testing"

	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
)

func TestCaptureDecisionTool_WithSession(t *testing.T) {
	tmp, err := os.MkdirTemp("", "brainstorm-decision-session-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	mgr := state.NewManager(tmp)
	eng := engine.NewEngine(tmp, db)

	// Pre-seed session so Handle gets past LoadSession
	_ = mgr.SaveSession(context.Background(), &models.Session{
		ProjectRoot: tmp,
		Metadata:    map[string]any{"standards": "use ADR format"},
	})

	tool := &CaptureDecisionTool{Manager: mgr, Engine: eng}
	input := DecisionInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Context: "Choose PostgreSQL over MySQL for ACID compliance requirements.",
		},
	}
	res, payload, err := tool.Handle(context.Background(), &mcp.CallToolRequest{}, input)
	_ = err
	_ = payload
	_ = res
}

func TestCaptureDecisionTool_SessionMetadataDecisions(t *testing.T) {
	tmp, _ := os.MkdirTemp("", "brainstorm-decision-meta-*")
	defer os.RemoveAll(tmp)
	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	mgr := state.NewManager(tmp)
	eng := engine.NewEngine(tmp, db)

	// Pre-seed with existing decisions list
	_ = mgr.SaveSession(context.Background(), &models.Session{
		ProjectRoot: tmp,
		Metadata:    map[string]any{"decisions": []any{"prior decision 1"}},
	})

	tool := &CaptureDecisionTool{Manager: mgr, Engine: eng}
	res, _, _ := tool.Handle(context.Background(), &mcp.CallToolRequest{}, DecisionInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Context: "Use Redis for session storage.",
		},
	})
	_ = res
}
