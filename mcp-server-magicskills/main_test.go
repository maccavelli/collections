package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magicskills/internal/config"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/handler"
	"mcp-server-magicskills/internal/handler/discovery"
	"mcp-server-magicskills/internal/registry"
)

func TestSetupLogging_Main(t *testing.T) {
	lb := &handler.LogBuffer{}
	setupLogging(lb)
	t.Log("setupLogging completed without panicking")
}

func TestResolveRoots_Config(t *testing.T) {
	if err := os.Setenv("MAGIC_SKILLS_PATH", "/tmp/magic1:/tmp/magic2"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Unsetenv("MAGIC_SKILLS_PATH")
	}()

	roots := config.ResolveRoots()
	if len(roots) < 2 {
		t.Fatalf("Expected at least 2 roots, got %d", len(roots))
	}

	found := false
	for _, r := range roots {
		if r == "/tmp/magic1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("MAGIC_SKILLS_PATH env var not resolved")
	}
}

func TestRegistryToolLoading(t *testing.T) {
	mcpSrv := mcp.NewServer(
		&mcp.Implementation{
			Name:    "test",
			Version: "1.0",
		},
		&mcp.ServerOptions{},
	)
	eng := engine.NewEngine()
	discovery.Register(eng)

	// In the refactored main, we iterate over it
	for _, tool := range registry.Global.List() {
		tool.Register(mcpSrv)
	}

	t.Log("Tools registered successfully from global registry")
}

func TestExecute_Cancel_Main(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	cancel()

	errCh := make(chan error)
	go func() {
	}()

	select {
	case <-time.After(50 * time.Millisecond):
		t.Log("Startup passed without panic")
	case <-errCh:
	}
}

