package handler

import (
	"context"
	"fmt"
	"strings"

	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/state"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func HandleAnalyzeProject(
	mgr *state.Manager, eng *engine.Engine,
) server.ToolHandlerFunc {
	return func(
		ctx context.Context, req mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		session, err := mgr.LoadSession(ctx)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("load session: %v", err),
			), nil
		}

		gaps, err := eng.AnalyzeDiscovery(ctx, path)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Errorf("analyze discovery: %w", err).Error(),
			), nil
		}

		session.Gaps = gaps
		if err := mgr.SaveSession(ctx, session); err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("save session: %v", err),
			), nil
		}

		truncated := false
		outGaps := session.Gaps
		const maxGaps = 10
		if len(outGaps) > maxGaps {
			outGaps = outGaps[:maxGaps]
			truncated = true
		}

		resp := map[string]interface{}{
			"status": session.Status,
			"gaps":   outGaps,
		}
		if truncated {
			resp["truncated"] = fmt.Sprintf(
				"%d more gaps omitted",
				len(session.Gaps)-maxGaps,
			)
		}
		return mcp.NewToolResultJSON(resp)
	}
}

func HandleSuggestNextStep(
	mgr *state.Manager, eng *engine.Engine,
) server.ToolHandlerFunc {
	return func(
		ctx context.Context, req mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		session, err := mgr.LoadSession(ctx)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("load session: %v", err),
			), nil
		}

		suggestion, err := eng.SuggestNextStep(
			ctx, session, path,
		)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Errorf("suggest next step: %w", err).Error(),
			), nil
		}
		return mcp.NewToolResultText(suggestion), nil
	}
}

// defaultLogLines is the maximum number of log lines
// returned when max_lines is not specified.
const defaultLogLines = 25

func HandleGetInternalLogs(
	logBuffer interface{ String() string },
) server.ToolHandlerFunc {
	return func(
		_ context.Context, req mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		maxLines := defaultLogLines
		if v := req.GetInt("max_lines", 0); v > 0 {
			maxLines = v
		}
		return mcp.NewToolResultText(
			tailLines(logBuffer.String(), maxLines),
		), nil
	}
}

// tailLines returns the last n lines of s. If s has
// fewer than n lines, the full string is returned.
func tailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	// Trim trailing empty element from final newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

func HandleDiscoverProject(
	mgr *state.Manager, eng *engine.Engine,
) server.ToolHandlerFunc {
	return func(
		ctx context.Context, req mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		session, err := mgr.LoadSession(ctx)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("load session: %v", err),
			), nil
		}

		resp, err := eng.DiscoverProject(ctx, path, session)
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Errorf("discover project: %w", err).Error(),
			), nil
		}

		if err := mgr.SaveSession(ctx, session); err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("save session: %v", err),
			), nil
		}

		return mcp.NewToolResultJSON(resp)
	}
}
