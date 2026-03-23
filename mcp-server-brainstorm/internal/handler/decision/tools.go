package decision

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/registry"
)

// CaptureDecisionTool handles decision ADR generation.
type CaptureDecisionTool struct {
	Engine *engine.Engine
}

func (t *CaptureDecisionTool) Metadata() mcp.Tool {
	return mcp.NewTool("capture_decision_logic",
		mcp.WithDescription("Generates a structured ADR capturing context and alternatives."),
		mcp.WithString("decision", mcp.Description("The decision being made"), mcp.Required()),
		mcp.WithString("alternatives", mcp.Description("The considered alternatives"), mcp.Required()),
	)
}

func (t *CaptureDecisionTool) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	decision, err := req.RequireString("decision")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("missing 'decision': %v", err)), nil
	}
	alternatives, err := req.RequireString("alternatives")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("missing 'alternatives': %v", err)), nil
	}

	slog.Info("executing decision capture")
	adr, err := t.Engine.CaptureDecisionLogic(ctx, decision, alternatives)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("capture decision: %v", err)), nil
	}
	return mcp.NewToolResultJSON(adr)
}

// Register adds the decision tools to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&CaptureDecisionTool{Engine: eng})
}
