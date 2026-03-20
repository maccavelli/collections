package handler

import (
	"context"
	"fmt"

	"mcp-server-brainstorm/internal/engine"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func HandleChallengeAssumption(
	eng *engine.Engine,
) server.ToolHandlerFunc {
	return func(
		ctx context.Context, req mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		design, err := req.RequireString("design")
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("missing 'design': %v", err),
			), nil
		}

		challenges, err := eng.ChallengeAssumption(
			ctx, design,
		)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("challenge assumption: %v", err),
			), nil
		}
		return mcp.NewToolResultJSON(challenges)
	}
}

func HandleAnalyzeEvolution(
	eng *engine.Engine,
) server.ToolHandlerFunc {
	return func(
		ctx context.Context, req mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		proposal, err := req.RequireString("proposal")
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("missing 'proposal': %v", err),
			), nil
		}

		result, err := eng.AnalyzeEvolution(ctx, proposal)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("analyze evolution: %v", err),
			), nil
		}
		return mcp.NewToolResultJSON(result)
	}
}

func HandleEvaluateQuality(
	eng *engine.Engine,
) server.ToolHandlerFunc {
	return func(
		ctx context.Context, req mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		design, err := req.RequireString("design")
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("missing 'design': %v", err),
			), nil
		}

		metrics, err := eng.EvaluateQualityAttributes(
			ctx, design,
		)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("evaluate quality: %v", err),
			), nil
		}
		return mcp.NewToolResultJSON(metrics)
	}
}

func HandleRedTeamReview(
	eng *engine.Engine,
) server.ToolHandlerFunc {
	return func(
		ctx context.Context, req mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		design, err := req.RequireString("design")
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("missing 'design': %v", err),
			), nil
		}

		challenges, err := eng.RedTeamReview(ctx, design)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("red team review: %v", err),
			), nil
		}
		return mcp.NewToolResultJSON(challenges)
	}
}

func HandleCritiqueDesign(
	eng *engine.Engine,
) server.ToolHandlerFunc {
	return func(
		ctx context.Context, req mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		design, err := req.RequireString("design")
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("missing 'design': %v", err),
			), nil
		}

		resp, err := eng.CritiqueDesign(ctx, design)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("critique design: %v", err),
			), nil
		}
		return mcp.NewToolResultJSON(resp)
	}
}
