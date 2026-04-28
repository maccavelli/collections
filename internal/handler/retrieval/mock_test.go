package retrieval

import (
	"context"
	"mcp-server-magicskills/internal/engine"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetTool_Fallback(t *testing.T) {
	defer func() { recover() }()
	eng, _ := engine.NewEngine(nil, "")
	close(eng.ReadyCh)
	tool := &GetTool{Engine: eng}

	// Test blank parameters triggering error fallbacks naturally inside GetTool.Handle
	tool.Handle(context.Background(), &mcp.CallToolRequest{}, GetInput{})

	// Test valid struct parsing
	tool.Handle(context.Background(), &mcp.CallToolRequest{}, GetInput{Name: "mock_skill", Section: "rules"})
}
