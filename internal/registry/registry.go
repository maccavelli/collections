package registry

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool defines the unified interface for MagicSkills MCP tools.
type Tool interface {
	// Name returns the unique name of the tool.
	Name() string
	// Register adds the tool to the provided MCP server.
	Register(s *mcp.Server)
}

// Registry manages a central collection of available MagicSkills tools.
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

// Get retrieves a tool by its name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	var list []Tool
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

