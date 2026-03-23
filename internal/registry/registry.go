package registry

import (
	"context"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tool defines the interface for an MCP tool.
type Tool interface {
	Metadata() mcp.Tool
	Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// Registry manages tool registration and retrieval.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// Global is the default registry instance.
var Global = &Registry{
	tools: make(map[string]Tool),
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Metadata().Name] = t
}

// Get finds a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}
