package registry

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MockTool struct {
	toolName string
}

func (m *MockTool) Name() string {
	return m.toolName
}

func (m *MockTool) Register(s *mcp.Server) {
}

func (m *MockTool) Handle(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	tool := &MockTool{toolName: "test_tool"}
	r.Register(tool)

	list := r.List()
	if len(list) != 1 {
		t.Errorf("expected list size 1, got %d", len(list))
	}
	if list[0].Name() != "test_tool" {
		t.Errorf("expected test_tool, got %s", list[0].Name())
	}
}
