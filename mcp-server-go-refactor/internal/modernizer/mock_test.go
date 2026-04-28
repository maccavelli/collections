package modernizer

import (
	"context"
	"mcp-server-go-refactor/internal/engine"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestModernizeHandle_Fallback(t *testing.T) {
	defer func() { recover() }()
	tool := &Tool{Engine: engine.NewEngine(nil)}
	tool.Handle(context.Background(), &mcp.CallToolRequest{}, ModernizeInput{})
}
