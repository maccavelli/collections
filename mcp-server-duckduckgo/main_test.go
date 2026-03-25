package main

import (
	"testing"

	"mcp-server-duckduckgo/internal/engine"
	"mcp-server-duckduckgo/internal/handler/media"
	"mcp-server-duckduckgo/internal/handler/search"
	"mcp-server-duckduckgo/internal/registry"
)



func TestRegistryToolLoading_Duck(t *testing.T) {
	eng := engine.NewSearchEngine()
	search.Register(eng)
	media.Register(eng)

	tools := registry.Global.List()
	if len(tools) != 5 {
		t.Errorf("expected 5 tools in registry, got %d", len(tools))
	}

	for _, tool := range tools {
		if tool.Name() == "" {
			t.Errorf("tool name is empty")
		}
	}
}
