package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
)

func TestDiscoverProjectTool_Handle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "brainstorm-discovery-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Add a sentinel to prevent walking up to system /tmp
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)

	mgr := state.NewManager(tmpDir)
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(tmpDir, db)
	tool := &DiscoverProjectTool{
		Manager: mgr,
		Engine:  eng,
	}

	ctx := context.Background()
	input := DiscoverInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target: tmpDir,
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
	if tool.Name() != "discover_project" {
		t.Errorf("expected discover_project, got %s", tool.Name())
	}
}

func TestRegister(t *testing.T) {
	mgr := state.NewManager(".")
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(".", db)

	Register(mgr, eng)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, &mcp.ServerOptions{})
	tool := &DiscoverProjectTool{Manager: mgr, Engine: eng}
	tool.Register(&util.MockSessionProvider{Srv: srv})
}

type mockRecallClient struct {
	recallEnabled bool
}

func (m *mockRecallClient) CallDatabaseTool(ctx context.Context, toolName string, arguments map[string]any) string {
	return "mock data"
}
func (m *mockRecallClient) AggregateSessionFromRecall(ctx context.Context, serverID, projectID string) (map[string]any, error) {
	return map[string]any{"key": "value"}, nil
}
func (m *mockRecallClient) SaveSession(ctx context.Context, sessionID, projectID string, data any) error {
	return nil
}
func (m *mockRecallClient) RecallEnabled() bool {
	return m.recallEnabled
}

func TestDiscoverProjectTool_Handle_Recall(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "brainstorm-discovery-recall-*")
	defer os.RemoveAll(tmpDir)
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)

	mgr := state.NewManager(tmpDir)
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(tmpDir, db)
	eng.ExternalClient = &mockRecallClient{recallEnabled: true}

	os.Setenv("MCP_ORCHESTRATOR_OWNED", "true")
	defer os.Unsetenv("MCP_ORCHESTRATOR_OWNED")

	tool := &DiscoverProjectTool{
		Manager: mgr,
		Engine:  eng,
	}

	ctx := context.Background()
	input := DiscoverInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target:    tmpDir,
			SessionID: "test-session",
		},
	}

	_, resp, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Errorf("Handle failed: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestDiscoverProjectTool_Handle_Errors(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "brainstorm-discovery-err-*")
	defer os.RemoveAll(tmpDir)
	mgr := state.NewManager(tmpDir)
	db, _ := buntdb.Open(":memory:")
	eng := engine.NewEngine(tmpDir, db)
	tool := &DiscoverProjectTool{Manager: mgr, Engine: eng}

	// 1. LoadSession error (via canceled context)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, _, _ := tool.Handle(ctx, &mcp.CallToolRequest{}, DiscoverInput{})
	if res.IsError == false {
		t.Error("expected error on canceled context")
	}

	// 2. DiscoverProject error
	ctx2 := context.Background()
	// DiscoverProject fails if path is missing or invalid
	res2, _, _ := tool.Handle(ctx2, &mcp.CallToolRequest{}, DiscoverInput{
		UniversalPipelineInput: models.UniversalPipelineInput{Target: "/non/existent"},
	})
	if res2.IsError == false {
		t.Error("expected error on non-existent target")
	}
}
