package sequentialthinking

import (
	"context"
	"testing"
	
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-sequential-thinking/internal/engine"
)

func TestRegister(t *testing.T) {
	eng := engine.NewEngine()
	Register(eng)
	tool := &SequentialThinkingTool{Engine: eng}
	if tool.Name() != "sequentialthinking" {
		t.Error("bad name")
	}
	
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "1.0"},
		&mcp.ServerOptions{},
	)
	tool.Register(srv)
	
	var input engine.ThoughtData
	req := &mcp.CallToolRequest{}
	
	// Test success
	input.Thought = "hello"
	input.ThoughtNumber = 1
	input.TotalThoughts = 1
	
	res, _, err := tool.Handle(context.Background(), req, input)
	if err != nil || res == nil {
		t.Error("Handle failed")
	}
}
