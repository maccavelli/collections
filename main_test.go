package main

import (
	"mcp-server-go-refactor/internal/handler/system"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/handler"
	"testing"
)

func TestMainScaffold_GoRefactor(t *testing.T) {
	if Version == "" {
		t.Fatal("Version should be set")
	}
}

func TestRegistryToolLoading_GoRefactor(t *testing.T) {
	buffer := &system.LogBuffer{}
	handler.RegisterAllTools(buffer)

	tools := registry.Global.List()
	if len(tools) == 0 {
		t.Errorf("expected tools to be registered, got 0")
	}

	// Should have 10 refactor tools + 1 system tool
	const expected = 11
	if len(tools) != expected {
		t.Errorf("expected %d tools, got %d", expected, len(tools))
	}
}
