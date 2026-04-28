package docgen

import (
	"context"
	"mcp-server-go-refactor/internal/engine"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDocgenHandle_Fallback(t *testing.T) {
	defer func() { recover() }()
	tool := &Tool{Engine: engine.NewEngine(nil)}
	tool.Handle(context.Background(), &mcp.CallToolRequest{}, DocInput{})
}
