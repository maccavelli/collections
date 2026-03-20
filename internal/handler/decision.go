package handler

import (
	"context"
	"fmt"

	"mcp-server-brainstorm/internal/engine"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func HandleCaptureDecision(
	eng *engine.Engine,
) server.ToolHandlerFunc {
	return func(
		ctx context.Context, req mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		decision, err := req.RequireString("decision")
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("missing 'decision': %v", err),
			), nil
		}
		alternatives, err := req.RequireString(
			"alternatives",
		)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("missing 'alternatives': %v", err),
			), nil
		}

		adr, err := eng.CaptureDecisionLogic(
			ctx, decision, alternatives,
		)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("capture decision: %v", err),
			), nil
		}
		return mcp.NewToolResultJSON(adr)
	}
}
