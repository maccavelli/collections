package discovery

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/registry"
	"mcp-server-brainstorm/internal/state"
)

// DiscoverProjectTool handles the unified discovery scan.
type DiscoverProjectTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

func (t *DiscoverProjectTool) Name() string {
	return "discover_project"
}

func (t *DiscoverProjectTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "SYSTEM ORIENTATION MANDATE: Conducts a comprehensive structural analysis of a project to map its architecture and identify missing critical components (tests, docs, configs). Call this FIRST for any new development or session to map architecture and debt. Cascades to clarify_requirements.",
	}, t.Handle)
}

type DiscoverInput struct {
	Path string `json:"path" jsonschema:"Optional absolute path to the project root."`
}

func (t *DiscoverProjectTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DiscoverInput) (*mcp.CallToolResult, any, error) {
	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("load session: %v", err))
		return res, nil, nil
	}

	resp, err := t.Engine.DiscoverProject(ctx, input.Path, session)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("save session: %v", err))
		return res, nil, nil
	}

	return &mcp.CallToolResult{}, resp, nil
}

// Register adds the discovery tools to the registry.
func Register(mgr *state.Manager, eng *engine.Engine) {
	registry.Global.Register(&DiscoverProjectTool{
		Manager: mgr,
		Engine:  eng,
	})
}
