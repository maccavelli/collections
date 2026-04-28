package discovery

import (
	"context"
	"mcp-server-brainstorm/internal/state"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDiscovery_EmptyConstraints(t *testing.T) {
	defer func() { recover() }()
	tool := &DiscoverProjectTool{Manager: state.NewManager("")}
	tool.Handle(context.Background(), &mcp.CallToolRequest{}, DiscoverInput{})
}
