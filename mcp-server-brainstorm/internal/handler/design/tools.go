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
		mcp.WithDescription("Subjects an architectural or technical design to a rigorous, multi-perspective review including Socratic inquiry for hidden assumptions, Red Team evaluation for failure modes, and a quality audit against industry best practices. This hardens your architecture before a single line of code is written, preventing costly downstream revisions. Use this for RFCs, design docs, or complex feature specifications."),
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
		mcp.WithDescription("Evaluates the blast radius and potential risks associated with a proposed architectural change or system expansion. It identifies upstream dependencies, potential regressions, and structural instabilities that could arise from the evolution. Use this when planning major refactors or adding high-impact features to assess feasibility and safety."),
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

// ClarifyRequirementsTool handles requirement grounding.
type ClarifyRequirementsTool struct {
	Engine *engine.Engine
}

func (t *ClarifyRequirementsTool) Metadata() mcp.Tool {
	return mcp.NewTool("clarify_requirements",
		mcp.WithDescription("Analyzes high-level requirements to detect architectural \"decision forks\" where ambiguity could lead to diverging implementations. It generates targeted Socratic questions that force a precise definition of constraints and intent. Use this during the initial scoping of a requirement to ensure the technical foundation matches the user's ultimate goal."),
		mcp.WithString("requirements", mcp.Description("The requirement text to analyze"), mcp.Required()),
	)
}

func (t *ClarifyRequirementsTool) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	requirements, err := req.RequireString("requirements")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("missing 'requirements': %v", err)), nil
	}

	slog.Info("executing requirement clarification")
	resp, err := t.Engine.ClarifyRequirements(ctx, requirements)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("clarify requirements: %v", err)), nil
	}
	return mcp.NewToolResultJSON(resp)
}

// Register adds the design tools to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&CritiqueDesignTool{Engine: eng})
	registry.Global.Register(&AnalyzeEvolutionTool{Engine: eng})
	registry.Global.Register(&ClarifyRequirementsTool{Engine: eng})
}
