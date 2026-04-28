package registry

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"testing"
)

type mockTool struct{ name string }

func (m *mockTool) Name() string           { return m.name }
func (m *mockTool) Register(s *mcp.Server) {}

func TestRegistry(t *testing.T) {
	Global.Register(&mockTool{name: "test1"})
	tools := Global.List()
	if len(tools) == 0 {
		t.Error("expected tool")
	}
}
