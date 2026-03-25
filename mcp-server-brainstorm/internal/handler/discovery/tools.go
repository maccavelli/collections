package discovery

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/registry"
	"mcp-server-brainstorm/internal/state"
)

// DiscoverProjectTool handles the unified discovery scan.
type DiscoverProjectTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

func (t *DiscoverProjectTool) Metadata() mcp.Tool {
	return mcp.NewTool("discover_project",
		mcp.WithDescription("Conducts a comprehensive structural analysis of a project to map its architecture and identify missing critical components (tests, docs, configs). This is essential for orienting developers in large codebases and identifying technical debt early. Use this at the start of any new session or when onboarding to a project to determine the most impactful next step."),
		mcp.WithString("path", mcp.Description("Optional absolute path to the project root.")),
	)
}

func (t *DiscoverProjectTool) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	slog.Info("executing discovery scan", "path", path)

	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("load session: %v", err)), nil
	}

	resp, err := t.Engine.DiscoverProject(ctx, path, session)
	if err != nil {
		return mcp.NewToolResultError(fmt.Errorf("discover project: %w", err).Error()), nil
	}

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("save session: %v", err)), nil
	}

	return mcp.NewToolResultJSON(resp)
}

// Register adds the discovery tools to the registry.
func Register(mgr *state.Manager, eng *engine.Engine) {
	registry.Global.Register(&DiscoverProjectTool{
		Manager: mgr,
		Engine:  eng,
	})
}
