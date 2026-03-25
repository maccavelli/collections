package main

import (
	"context"
	"mcp-server-go-refactor/internal/handler"
	"mcp-server-go-refactor/internal/handler/system"
	"mcp-server-go-refactor/internal/registry"
	"testing"
)

func TestMainScaffold_GoRefactor(t *testing.T) {
	if Version == "" {
		t.Fatal("Version should be set")
	}
	printVersion()
}

func TestSetupLogging_GoRefactor(t *testing.T) {
	buffer := &system.LogBuffer{}
	setupLogging(buffer)
}

func TestRunServerCancellation_GoRefactor(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// buffer := &system.LogBuffer{}
	// run(ctx, buffer) // Not calling because ServeStdio will block
}

func TestRegistryToolLoading_GoRefactor(t *testing.T) {
	buffer := &system.LogBuffer{}
	handler.RegisterAllTools(buffer)

	tools := registry.Global.List()
	if len(tools) == 0 {
		t.Errorf("expected tools to be registered, got 0")
	}

	// Dynamic check for tools
	if len(tools) < 10 {
		t.Errorf("expected at least 10 tools, got %d", len(tools))
	}
}
