package registry

import (
	"sync"

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
	mu    sync.RWMutex
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Get retrieves a tool by its name.
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
	var list []Tool
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}
