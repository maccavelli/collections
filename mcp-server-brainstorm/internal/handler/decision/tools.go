package decision

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/registry"
)

// CaptureDecisionTool handles decision ADR generation.
type CaptureDecisionTool struct {
	Engine *engine.Engine
}

func (t *CaptureDecisionTool) Name() string {
	return "capture_decision_logic"
}

func (t *CaptureDecisionTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "ADR SYNTHESIS: Formalizes the architectural decision-making process by generating structured ADRs. Call this immediately after making a key design choice to preserve institutional knowledge and the 'why' behind the code.",
	}, t.Handle)
}

type DecisionInput struct {
	Decision     string `json:"decision" jsonschema:"The decision being made"`
	Alternatives string `json:"alternatives" jsonschema:"The considered alternatives"`
}

func (t *CaptureDecisionTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DecisionInput) (*mcp.CallToolResult, any, error) {
	adr, err := t.Engine.CaptureDecisionLogic(ctx, input.Decision, input.Alternatives)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, adr, nil
}

// Register adds the decision tools to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&CaptureDecisionTool{Engine: eng})
}
