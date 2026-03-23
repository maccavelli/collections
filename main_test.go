package main

import (
	"mcp-server-brainstorm/internal/config"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/handler/decision"
	"mcp-server-brainstorm/internal/handler/design"
	"mcp-server-brainstorm/internal/handler/discovery"
	"mcp-server-brainstorm/internal/handler/system"
	"mcp-server-brainstorm/internal/registry"
	"mcp-server-brainstorm/internal/state"
	"testing"
)

func TestLogBuffer_Trimming_Brain(t *testing.T) {
	lb := &system.LogBuffer{}
	
	msg := "test log message\n"
	_, _ = lb.Write([]byte(msg))
	if lb.String() != msg {
		t.Errorf("want '%s', got '%s'", msg, lb.String())
	}

	// Trimming logic
	data := make([]byte, config.LogBufferLimit+100)
	for i := range data {
		data[i] = 'a'
		if i%100 == 0 {
			data[i] = '\n'
		}
	}
	_, _ = lb.Write(data)
	if len(lb.String()) > config.LogBufferLimit {
		t.Errorf("buffer length %d exceeds limit after trimming", len(lb.String()))
	}
}

func TestVersionReporting_Brain(t *testing.T) {
	printVersion()
}

func TestRegistryToolLoading_Brain(t *testing.T) {
	wd := "."
	mgr := state.NewManager(wd)
	eng := engine.NewEngine(wd)
	buffer := &system.LogBuffer{}

	discovery.Register(mgr, eng)
	design.Register(eng)
	decision.Register(eng)
	system.Register(buffer)

	tools := registry.Global.List()
	if len(tools) == 0 {
		t.Error("expected tools to be registered in global registry")
	}

	// Verify specific tools
	if _, ok := registry.Global.Get("discover_project"); !ok {
		t.Error("discover_project tool not found in registry")
	}
	if _, ok := registry.Global.Get("get_internal_logs"); !ok {
		t.Error("get_internal_logs tool not found in registry")
	}
}
