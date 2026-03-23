package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
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
		meta := tool.Metadata()
		if meta.Name == "" {
			t.Errorf("tool metadata name is empty")
		}
	}
}

func TestSearchTool_Handle(t *testing.T) {
	eng := engine.NewSearchEngine()
	search.Register(eng)

	tool, ok := registry.Global.Get("ddg_search_web")
	if !ok {
		t.Fatal("ddg_search_web tool not registered")
	}

	t.Run("missing_query", func(t *testing.T) {
		req := mcp.CallToolRequest{}
		req.Params.Name = "ddg_search_web"
		req.Params.Arguments = map[string]interface{}{}

		res, err := tool.Handle(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if !res.IsError {
			t.Error("expected error result due to missing query")
		}
	})
}
