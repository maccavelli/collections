package registry

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool defines the interface for an MCP tool compatible with the official SDK.
type Tool interface {
	Name() string
	Register(s *mcp.Server)
}

// Registry manages tool registration.
type Registry struct {
	tools map[string]Tool
}

// Global is the default registry instance.
var Global = NewRegistry()

// NewRegistry initializes an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	var list []Tool
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}
