package security

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
)

func TestThreatModelerTool_Handle(t *testing.T) {
	mgr := state.NewManager(".")
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(".", db)
	tool := &ThreatModelerTool{Manager: mgr, Engine: eng}

	ctx := context.Background()

	// Case 1: Standalone Success
	req := &mcp.CallToolRequest{}
	input := ThreatModelerInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			SessionID: "test-session",
			Target:    ".",
			Context:   "test architecture",
		},
	}

	_, result, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	resp := result.(models.ThreatModelResponse)
	if resp.Data.Narrative == "" {
		t.Error("expected narrative in response")
	}

	// Case 2: Recall enabled simulation
	t.Setenv("MCP_ORCHESTRATOR_OWNED", "true")
	mockClient := &mockRecallClient{}
	eng.ExternalClient = mockClient

	session, _ := mgr.LoadSession(ctx)
	session.ProjectRoot = "."
	mgr.SaveSession(ctx, session)

	_, result2, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Recall path failed: %v", err)
	}
	if !mockClient.aggregateCalled {
		t.Error("expected AggregateSessionFromRecall to be called")
	}
	if !mockClient.saveCalled {
		t.Error("expected SaveSession to be called")
	}

	// Check wrapped return data (since SaveSession succeeded)
	wrapped := result2.(struct {
		Summary string `json:"summary"`
		Data    any    `json:"data,omitempty"`
	})
	if wrapped.Summary == "" {
		t.Error("expected summary in wrapped response")
	}

	// Case 3: SaveSession failure simulation
	mockClient.saveError = fmt.Errorf("recall save failed")
	_, result3, _ := tool.Handle(ctx, req, input)
	resp3 := result3.(models.ThreatModelResponse)
	// If save fails, it returns the standard response with an updated summary
	if resp3.Data.Narrative == "" {
		t.Error("expected narrative even if recall save fails")
	}
}

func TestThreatModelerTool_Name(t *testing.T) {
	tool := &ThreatModelerTool{}
	if tool.Name() != "threat_model_auditor" {
		t.Errorf("expected threat_model_auditor, got %s", tool.Name())
	}
}

func TestThreatModelerTool_Register(t *testing.T) {
	s := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "1.0.0"},
		&mcp.ServerOptions{},
	)
	sp := &util.MockSessionProvider{Srv: s}
	tool := &ThreatModelerTool{}
	tool.Register(sp)
}

func TestRegister(t *testing.T) {
	Register(nil, nil)
}

type mockRecallClient struct {
	aggregateCalled bool
	saveCalled      bool
	saveError       error
}

func (m *mockRecallClient) RecallEnabled() bool { return true }
func (m *mockRecallClient) AggregateSessionFromRecall(ctx context.Context, server, project string) (map[string]any, error) {
	m.aggregateCalled = true
	return map[string]any{"test": "data"}, nil
}
func (m *mockRecallClient) SaveSession(ctx context.Context, sessionID, nonce string, data any) error {
	m.saveCalled = true
	return m.saveError
}
func (m *mockRecallClient) CallDatabaseTool(ctx context.Context, tool string, arguments map[string]any) string {
	return ""
}
func (m *mockRecallClient) PublishSessionToRecall(ctx context.Context, sessionID, project, outcome, nonce, tool, standards string, metadata map[string]any) error {
	return nil
}
func (m *mockRecallClient) LoadCrossSessionFromRecall(ctx context.Context, peer, project string) (map[string]any, error) {
	return nil, nil
}
