package system

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetInternalLogsTool_Handle(t *testing.T) {
	buffer := &LogBuffer{}
	buffer.Write([]byte("line1\nline2\nline3\n"))
	tool := &GetInternalLogsTool{Buffer: buffer}

	ctx := context.Background()
	input := LogsInput{
		MaxLines: 2,
	}

	res, _, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Errorf("Handle failed: %v", err)
	}
	txt := res.Content[0].(*mcp.TextContent).Text
	if txt != "line2\nline3" {
		t.Errorf("expected last 2 lines, got %q", txt)
	}

	if tool.Name() != "get_internal_logs" {
		t.Errorf("expected get_internal_logs, got %s", tool.Name())
	}
}

func TestGetInternalLogsTool_Register(t *testing.T) {
	buffer := &LogBuffer{}
	Register(buffer)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, &mcp.ServerOptions{})
	tool := &GetInternalLogsTool{Buffer: buffer}
	tool.Register(srv)
}
