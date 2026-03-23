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
		mcp.WithDescription("Performs a unified discovery scan, identifying gaps and suggesting the next logical step."),
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
