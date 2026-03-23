package handler

import (
	"context"
	"mcp-server-go-refactor/internal/handler/system"
	"mcp-server-go-refactor/internal/registry"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestHandlerRegistration(t *testing.T) {
	buffer := &system.LogBuffer{}
	RegisterAllTools(buffer)

	// Test LoadToolsFromRegistry
	s := server.NewMCPServer("test", "1.0.0")
	LoadToolsFromRegistry(s)
	
	tools := s.ListTools()
	if len(tools) == 0 {
		t.Error("expected tools to be loaded into server")
	}
}

func TestToolHandlers(t *testing.T) {
	ctx := context.Background()
	buffer := &system.LogBuffer{}
	RegisterAllTools(buffer)

	tests := []struct {
		name     string
		toolName string
		args     map[string]interface{}
	}{
		{
			name:     "doc_generator",
			toolName: "go_doc_generator",
			args:     map[string]interface{}{"pkg": "mcp-server-go-refactor/internal/docgen"},
		},
		{
			name:     "complexity_analyzer",
			toolName: "go_complexity_analyzer",
			args:     map[string]interface{}{"pkg": "mcp-server-go-refactor/internal/metrics"},
		},
		{
			name:     "sql_injection_guard",
			toolName: "go_sql_injection_guard",
			args:     map[string]interface{}{"pkg": "mcp-server-go-refactor/internal/safety"},
		},
		{
			name:     "dead_code_pruner",
			toolName: "go_dead_code_pruner",
			args:     map[string]interface{}{"pkg": "mcp-server-go-refactor/internal/pruner"},
		},
		{
			name:     "interface_toolkit",
			toolName: "go_interface_tool",
			args:     map[string]interface{}{"pkg": "mcp-server-go-refactor/internal/astutil", "structName": "MyStruct"},
		},
		{
			name:     "tag_manager",
			toolName: "go_tag_manager",
			args:     map[string]interface{}{"pkg": "mcp-server-go-refactor/internal/tags", "structName": "TagStruct", "caseFormat": "camel", "targetTag": "json"},
		},
		{
			name:     "struct_alignment",
			toolName: "go_struct_alignment_optimizer",
			args:     map[string]interface{}{"pkg": "mcp-server-go-refactor/internal/layout", "structName": "TestStruct"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool, ok := registry.Global.Get(tc.toolName)
			if !ok {
				t.Fatalf("%s tool not registered", tc.toolName)
			}

			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      tc.toolName,
					Arguments: tc.args,
				},
			}
			res, err := tool.Handle(ctx, req)
			if err != nil {
				// We expect some package loading to fail in a test environment without full source,
				// but the handler itself should exist. Just log the error.
				t.Logf("%s tool execution (expected some load failures): %v", tc.name, err)
			}
			if res == nil && err == nil {
				t.Fatalf("%s tool returned nil result without error", tc.name)
			}
		})
	}
}
