package registry

import (
	"mcp-server-brainstorm/internal/util"
	"testing"
)

type mockTool struct {
	name string
}

func (m *mockTool) Name() string                    { return m.name }
func (m *mockTool) Register(s util.SessionProvider) {}

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
