package design

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/registry"
)

// CritiqueDesignTool handles design assessment.
type CritiqueDesignTool struct {
	Engine *engine.Engine
}

func (t *CritiqueDesignTool) Metadata() mcp.Tool {
	return mcp.NewTool("critique_design",
		mcp.WithDescription("Provides a consolidated, multi-dimensional assessment of a design (Socratic, Red Team, Quality)."),
		mcp.WithString("design", mcp.Description("The design text to critique"), mcp.Required()),
	)
}

func (t *CritiqueDesignTool) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	design, err := req.RequireString("design")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("missing 'design': %v", err)), nil
	}

	slog.Info("executing design critique")
	resp, err := t.Engine.CritiqueDesign(ctx, design)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("critique design: %v", err)), nil
	}
	return mcp.NewToolResultJSON(resp)
}

// AnalyzeEvolutionTool handles risk identification in changes.
type AnalyzeEvolutionTool struct {
	Engine *engine.Engine
}

func (t *AnalyzeEvolutionTool) Metadata() mcp.Tool {
	return mcp.NewTool("analyze_evolution",
		mcp.WithDescription("Identifies risks in proposed project changes."),
		mcp.WithString("proposal", mcp.Description("The proposed change or extension"), mcp.Required()),
	)
}

func (t *AnalyzeEvolutionTool) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	proposal, err := req.RequireString("proposal")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("missing 'proposal': %v", err)), nil
	}

	slog.Info("executing evolution analysis")
	result, err := t.Engine.AnalyzeEvolution(ctx, proposal)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("analyze evolution: %v", err)), nil
	}
	return mcp.NewToolResultJSON(result)
}

// Register adds the design tools to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&CritiqueDesignTool{Engine: eng})
	registry.Global.Register(&AnalyzeEvolutionTool{Engine: eng})
}
