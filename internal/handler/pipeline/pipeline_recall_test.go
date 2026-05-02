package pipeline

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
)

type mockRecallClient struct{}

func (m *mockRecallClient) RecallEnabled() bool { return true }
func (m *mockRecallClient) SaveSession(ctx context.Context, sessionID, project string, data any) error {
	return nil
}
func (m *mockRecallClient) AggregateSessionFromRecall(ctx context.Context, server, project string) (map[string]any, error) {
	return map[string]any{"stride": "safe"}, nil
}
func (m *mockRecallClient) CallDatabaseTool(ctx context.Context, toolName string, arguments map[string]any) string {
	return "OK"
}

func TestAporiaEngineTool_RecallEnabled(t *testing.T) {
	// Set orchestrator mode
	t.Setenv("MCP_ORCHESTRATOR_OWNED", "true")

	mgr := state.NewManager(".")
	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	eng := engine.NewEngine(".", db)
	// Inject mock recall client directly (it satisfies RecallClient interface)
	eng.ExternalClient = &mockRecallClient{}

	tool := &AporiaEngineTool{Manager: mgr, Engine: eng}

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	input := AporiaInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			SessionID: "recall-session",
			Target:    ".",
		},
	}

	session, _ := mgr.LoadSession(ctx)
	session.ProjectRoot = "."
	session.Metadata["thesis_document"] = models.ThesisDocument{
		Data: struct {
			Narrative string                   `json:"narrative"`
			Pillars   []models.DialecticPillar `json:"pillars"`
			Standards string                   `json:"standards,omitempty"`
		}{
			Narrative: "Secure design",
			Pillars:   []models.DialecticPillar{{Name: "Security", Score: 8}},
		},
	}
	session.Metadata["counter_thesis"] = models.CounterThesisReport{
		Pillars: []models.DialecticPillar{{Name: "Security", Score: 4}},
	}
	mgr.SaveSession(ctx, session)

	_, result, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	report := result.(models.AporiaReport)
	if report.SafePathVerdict == "" {
		t.Error("expected SafePathVerdict with recall enabled")
	}
}
