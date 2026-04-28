package registry

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mockTool struct {
	name string
}

// Name returns the mock tool name.
func (m *mockTool) Name() string { return m.name }
// Register implements the Tool interface for testing.
func (m *mockTool) Register(s *mcp.Server) {}

// TestRegistry verifies the basic add and list behaviors of the tool registry.
func TestRegistry(t *testing.T) {
	reg := &Registry{
		tools: make(map[string]Tool),
	}

	t1 := &mockTool{name: "t1"}
	reg.Register(t1)

	tools := reg.List()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name() != "t1" {
		t.Errorf("expected t1, got %s", tools[0].Name())
	}
}
