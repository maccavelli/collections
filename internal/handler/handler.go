// Package handler provides functionality for the handler subsystem.
package handler

import (
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/handler/analytics"
	"mcp-server-brainstorm/internal/handler/decision"
	"mcp-server-brainstorm/internal/handler/design"
	"mcp-server-brainstorm/internal/handler/discovery"
	"mcp-server-brainstorm/internal/handler/pipeline"
	"mcp-server-brainstorm/internal/handler/security"
	"mcp-server-brainstorm/internal/handler/system"
	"mcp-server-brainstorm/internal/registry"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
)

// RegisterAllTools centralizes the registration of all brainstorm domain tools.
func RegisterAllTools(mgr *state.Manager, eng *engine.Engine, buffer *system.LogBuffer) {
	discovery.Register(mgr, eng)
	design.Register(mgr, eng)
	decision.Register(mgr, eng)
	system.Register(buffer)
	analytics.Register(mgr, eng)
	security.Register(mgr, eng)
	pipeline.Register(mgr, eng)
}

// LoadToolsFromRegistry maps registered tools to the MCP server instance.
func LoadToolsFromRegistry(s util.SessionProvider) int {
	tools := registry.Global.List()
	for _, t := range tools {
		t.Register(s)
	}
	return len(tools)
}
