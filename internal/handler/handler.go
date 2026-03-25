package handler

import (
	"mcp-server-go-refactor/internal/astutil"
	"mcp-server-go-refactor/internal/coverage"
	"mcp-server-go-refactor/internal/dependency"
	"mcp-server-go-refactor/internal/docgen"
	"mcp-server-go-refactor/internal/graph"
	"mcp-server-go-refactor/internal/handler/system"
	"mcp-server-go-refactor/internal/layout"
	"mcp-server-go-refactor/internal/metrics"
	"mcp-server-go-refactor/internal/modernizer"
	"mcp-server-go-refactor/internal/pruner"
	contextanalysis "mcp-server-go-refactor/internal/analysis/context"
	interfaceanalysis "mcp-server-go-refactor/internal/analysis/interface"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/safety"
	"mcp-server-go-refactor/internal/tags"

	"github.com/mark3labs/mcp-go/server"
)

// RegisterAllTools centralizes the registration of all refactoring domain tools.
func RegisterAllTools(buffer *system.LogBuffer) {
	astutil.Register()
	coverage.Register()
	dependency.Register()
	docgen.Register()
	graph.Register()
	layout.Register()
	metrics.Register()
	modernizer.Register()
	pruner.Register()
	safety.Register()
	tags.Register()
	contextanalysis.Register()
	interfaceanalysis.Register()
	system.Register(buffer)
}

// LoadToolsFromRegistry maps registered tools to the MCP server instance.
func LoadToolsFromRegistry(s *server.MCPServer) {
	for _, t := range registry.Global.List() {
		s.AddTool(t.Metadata(), t.Handle)
	}
}
