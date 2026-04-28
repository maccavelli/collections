package system

import (
	"context"
	"mcp-server-magicskills/internal/engine"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAllHandlers_Fallback(t *testing.T) {
	defer func() { recover() }()
	eng, _ := engine.NewEngine(nil, "")
	close(eng.ReadyCh)

	t1 := &ValidateDepsTool{Engine: eng}
	t1.Handle(context.Background(), &mcp.CallToolRequest{}, ValidateDepsInput{})

	t4 := &HealthTool{Engine: eng}
	t4.Handle(context.Background(), &mcp.CallToolRequest{}, HealthInput{})
}
