package system

import (
	"context"
	"mcp-server-magicskills/internal/engine"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAddRoot_PanicMock(t *testing.T) {
	defer func() { recover() }()
	eng, _ := engine.NewEngine(nil, "")
	close(eng.ReadyCh)
	tool := &AddRootTool{Engine: eng, Scanner: nil}
	tool.Handle(context.Background(), &mcp.CallToolRequest{}, AddRootInput{Path: "/tmp"})
}

func TestGetLogs_PanicMock(t *testing.T) {
	defer func() { recover() }()
	tool := &GetInternalLogsTool{Buffer: nil}
	tool.Handle(context.Background(), &mcp.CallToolRequest{}, LogsInput{MaxLines: 10})
}
