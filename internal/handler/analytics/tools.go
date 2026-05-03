// Package analytics provides functionality for the analytics subsystem.
package analytics

import (
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/registry"
	"mcp-server-brainstorm/internal/state"
)

// Register adds the analytics tools to the global registry.
func Register(mgr *state.Manager, eng *engine.Engine) {
	registry.Global.Register(&GenerateReportTool{Manager: mgr, Engine: eng})
}
