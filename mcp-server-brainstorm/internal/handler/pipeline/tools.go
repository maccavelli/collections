package pipeline

import (
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/registry"
	"mcp-server-brainstorm/internal/state"
)

// Register adds the pipeline tools to the brainstorm registry.
func Register(mgr *state.Manager, eng *engine.Engine) {
	registry.Global.Register(&AporiaEngineTool{Manager: mgr, Engine: eng})
	registry.Global.Register(&ASTProbeTool{Manager: mgr, Engine: eng})
	registry.Global.Register(&ComplexityForecasterTool{Manager: mgr, Engine: eng})
}
