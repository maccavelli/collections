package registry

import (
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool defines the interface for an MCP tool compatible with the official SDK.
type Tool interface {
	Name() string
	Register(s *mcp.Server)
}

// Registry manages tool registration.
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
	r.tools[t.Name()] = t
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
