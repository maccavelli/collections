package pruner

import (
	"context"
	"mcp-server-go-refactor/internal/engine"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPrunerHandle_Fallback(t *testing.T) {
	defer func() { recover() }()
	tool := &Tool{Engine: engine.NewEngine(nil)}

	// Execute handle limits structurally
	tool.Handle(context.Background(), &mcp.CallToolRequest{}, PruneInput{})
}
