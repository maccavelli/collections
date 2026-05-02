package handler

import (
	contextanalysis "mcp-server-go-refactor/internal/analysis/context"
	interfaceanalysis "mcp-server-go-refactor/internal/analysis/interface"
	memoryanalysis "mcp-server-go-refactor/internal/analysis/memory"
	"mcp-server-go-refactor/internal/astsuite"
	"mcp-server-go-refactor/internal/astutil"
	"mcp-server-go-refactor/internal/coverage"
	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/graph"
	"mcp-server-go-refactor/internal/handler/pipeline"
	"mcp-server-go-refactor/internal/handler/system"
	"mcp-server-go-refactor/internal/pruner"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/safety"
	"mcp-server-go-refactor/internal/suggestfixes"
	"mcp-server-go-refactor/internal/tags"
	"mcp-server-go-refactor/internal/util"
)

// RegisterAllTools centralizes the registration of all refactoring domain tools.
func RegisterAllTools(eng *engine.Engine, buffer *system.LogBuffer) {
	astutil.Register(eng)
	coverage.Register(eng)
	astsuite.Register(eng)
	graph.Register(eng)
	pruner.Register(eng)
	safety.Register(eng)
	tags.Register(eng)
	contextanalysis.Register(eng)
	interfaceanalysis.Register(eng)
	memoryanalysis.Register(eng)
	suggestfixes.Register(eng)
	pipeline.Register(eng)
	system.Register(buffer)
}

// LoadToolsFromRegistry maps registered tools to the MCP server instance.
func LoadToolsFromRegistry(s util.SessionProvider) int {
	tools := registry.Global.List()
	for _, t := range tools {
		t.Register(s)
	}
	return len(tools)
}
