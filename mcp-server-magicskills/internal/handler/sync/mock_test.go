package sync

import (
	"context"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/scanner"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSyncHandle_Fallback(t *testing.T) {
	defer func() { recover() }()
	eng, _ := engine.NewEngine(nil, "")
	close(eng.ReadyCh)
	scn, _ := scanner.NewScanner([]string{})

	tool := &SyncTool{Engine: eng, Scanner: scn}
	tool.Handle(context.Background(), &mcp.CallToolRequest{}, SyncInput{})
}
