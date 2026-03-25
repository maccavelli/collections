package design

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/registry"
)

// CritiqueDesignTool handles design assessment.
type CritiqueDesignTool struct {
	Engine *engine.Engine
}

func (t *CritiqueDesignTool) Name() string {
	return "critique_design"
}

func (t *CritiqueDesignTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "HARDENING AUDIT: Subjects an architectural design to a rigorous, multi-perspective review. Call this BEFORE implementation to prevent regressions and identify hidden assumptions. Cascades to analyze_evolution.",
	}, t.Handle)
}

type DesignInput struct {
	Design string `json:"design" jsonschema:"The design text to critique"`
}

func (t *CritiqueDesignTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DesignInput) (*mcp.CallToolResult, any, error) {
	resp, err := t.Engine.CritiqueDesign(ctx, input.Design)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, resp, nil
}

// AnalyzeEvolutionTool handles risk identification in changes.
type AnalyzeEvolutionTool struct {
	Engine *engine.Engine
}

func (t *AnalyzeEvolutionTool) Name() string {
	return "analyze_evolution"
}

func (t *AnalyzeEvolutionTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "RISK ASSESSMENT / BLAST RADIUS: Evaluates potential risks associated with proposed architectural changes. Call this during planning to assess feasibility and safety. Cascades to sequential_thinking for planning.",
	}, t.Handle)
}

type EvolutionInput struct {
	Proposal string `json:"proposal" jsonschema:"The proposed change or extension"`
}

func (t *AnalyzeEvolutionTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input EvolutionInput) (*mcp.CallToolResult, any, error) {
	result, err := t.Engine.AnalyzeEvolution(ctx, input.Proposal)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, result, nil
}

// ClarifyRequirementsTool handles requirement grounding.
type ClarifyRequirementsTool struct {
	Engine *engine.Engine
}

func (t *ClarifyRequirementsTool) Name() string {
	return "clarify_requirements"
}

func (t *ClarifyRequirementsTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "REQUIREMENTS GROUNDING: Analyzes high-level requirements to detect architectural ambiguity. Call this after discovery to ensure the technical foundation matches the goal. Cascades to critique_design.",
	}, t.Handle)
}

type RequirementsInput struct {
	Requirements string `json:"requirements" jsonschema:"The requirement text to analyze"`
}

func (t *ClarifyRequirementsTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input RequirementsInput) (*mcp.CallToolResult, any, error) {
	resp, err := t.Engine.ClarifyRequirements(ctx, input.Requirements)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, resp, nil
}

// Register adds the design tools to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&CritiqueDesignTool{Engine: eng})
	registry.Global.Register(&AnalyzeEvolutionTool{Engine: eng})
	registry.Global.Register(&ClarifyRequirementsTool{Engine: eng})
}
