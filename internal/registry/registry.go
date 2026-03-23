package registry

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tool defines the interface for all static analysis refactoring tools.
type Tool interface {
	// Metadata returns the MCP tool specification.
	Metadata() mcp.Tool
	// Handle executes the tool logic with the provided context and request.
	Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// Registry manages a central collection of available MCP tools.
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
	r.tools[tool.Metadata().Name] = tool
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
