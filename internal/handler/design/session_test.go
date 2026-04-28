package design

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

// helpers to seed a session in a tmp directory
func seed(t *testing.T) (string, *state.Manager, *engine.Engine) {
	t.Helper()
	tmp, err := os.MkdirTemp("", "brainstorm-design-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })

	db, _ := buntdb.Open(":memory:")
	t.Cleanup(func() { db.Close() })

	mgr := state.NewManager(tmp)
	eng := engine.NewEngine(tmp, db)

	// Pre-write a session to disk so LoadSession succeeds
	_ = mgr.SaveSession(context.Background(), &models.Session{
		ProjectRoot: tmp,
		Metadata:    map[string]any{"standards": "use SOLID principles"},
	})
	return tmp, mgr, eng
}

func TestCritiqueDesignTool_WithSession(t *testing.T) {
	_, mgr, eng := seed(t)
	tool := &CritiqueDesignTool{Manager: mgr, Engine: eng}
	input := DesignInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Context: "Use a shared database for all microservices.",
		},
	}
	res, payload, err := tool.Handle(context.Background(), &mcp.CallToolRequest{}, input)
	_ = err
	_ = payload
	_ = res
}

func TestAnalyzeEvolutionTool_WithSession(t *testing.T) {
	_, mgr, eng := seed(t)
	tool := &AnalyzeEvolutionTool{Manager: mgr, Engine: eng}
	input := EvolutionInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Context: "Replace synchronous REST calls with Kafka event streaming.",
		},
	}
	res, payload, err := tool.Handle(context.Background(), &mcp.CallToolRequest{}, input)
	_ = err
	_ = payload
	_ = res
}

func TestClarifyRequirementsTool_WithSession(t *testing.T) {
	_, mgr, eng := seed(t)
	tool := &ClarifyRequirementsTool{Manager: mgr, Engine: eng}
	input := RequirementsInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Context: "The system must handle 100k rps with less than 1ms latency.",
		},
	}
	res, payload, err := tool.Handle(context.Background(), &mcp.CallToolRequest{}, input)
	_ = err
	_ = payload
	_ = res
}
