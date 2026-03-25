package design

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
)

func TestCritiqueDesignTool_Handle(t *testing.T) {
	eng := engine.NewEngine(".")
	tool := &CritiqueDesignTool{
		Engine: eng,
	}

	ctx := context.Background()
	input := DesignInput{
		Design: "Use a centralized PostgreSQL database for all microservices.",
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
	eng := engine.NewEngine(".")
	tool := &AnalyzeEvolutionTool{
		Engine: eng,
	}

	ctx := context.Background()
	input := EvolutionInput{
		Proposal: "Add a caching layer with Redis to the API.",
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
	eng := engine.NewEngine(".")
	tool := &ClarifyRequirementsTool{
		Engine: eng,
	}

	ctx := context.Background()
	input := RequirementsInput{
		Requirements: "I want a high-performance system for real-time analytics.",
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
