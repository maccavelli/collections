package design

import (
	"context"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
	"testing"

	"mcp-server-brainstorm/internal/engine"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
)

func TestCritiqueDesignTool_Handle(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(".", db)
	mgr := state.NewManager("/tmp/test")
	tool := &CritiqueDesignTool{
		Manager: mgr,
		Engine:  eng,
	}

	ctx := context.Background()
	input := DesignInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Context: "Use a centralized PostgreSQL database for all microservices.",
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
	if tool.Name() != "critique_design" {
		t.Errorf("expected critique_design, got %s", tool.Name())
	}
}

func TestAnalyzeEvolutionTool_Handle(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(".", db)
	mgr := state.NewManager("/tmp/test")
	tool := &AnalyzeEvolutionTool{
		Manager: mgr,
		Engine:  eng,
	}

	ctx := context.Background()
	input := EvolutionInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Context: "Add a caching layer with Redis to the API.",
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
	if tool.Name() != "analyze_evolution" {
		t.Errorf("expected analyze_evolution, got %s", tool.Name())
	}
}

func TestClarifyRequirementsTool_Handle(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(".", db)
	mgr := state.NewManager("/tmp/test")
	tool := &ClarifyRequirementsTool{
		Manager: mgr,
		Engine:  eng,
	}

	ctx := context.Background()
	input := RequirementsInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Context: "I want a high-performance system for real-time analytics.",
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
	if tool.Name() != "clarify_requirements" {
		t.Errorf("expected clarify_requirements, got %s", tool.Name())
	}
}

func TestRegister(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	mgr := state.NewManager("/tmp/test")
	eng := engine.NewEngine(".", db)
	Register(mgr, eng)

	srv := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "1.0"},
		&mcp.ServerOptions{},
	)
	tool1 := &CritiqueDesignTool{Manager: mgr, Engine: eng}
	tool1.Register(&util.MockSessionProvider{Srv: srv})
	tool2 := &AnalyzeEvolutionTool{Manager: mgr, Engine: eng}
	tool2.Register(&util.MockSessionProvider{Srv: srv})
	tool3 := &ClarifyRequirementsTool{Manager: mgr, Engine: eng}
	tool3.Register(&util.MockSessionProvider{Srv: srv})
}
