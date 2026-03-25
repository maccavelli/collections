package registry

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

type MockTool struct {
	Name string
}

func (m *MockTool) Metadata() mcp.Tool {
	return mcp.NewTool(m.Name)
}

func (m *MockTool) Handle(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return nil, nil
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	tool := &MockTool{Name: "test_tool"}
	r.Register(tool)

	got, ok := r.Get("test_tool")
	if !ok {
		t.Fatal("expected tool not found")
	}
	if got.Metadata().Name != "test_tool" {
		t.Errorf("expected test_tool, got %s", got.Metadata().Name)
	}

	list := r.List()
	if len(list) != 1 {
		t.Errorf("expected list size 1, got %d", len(list))
	}
}
