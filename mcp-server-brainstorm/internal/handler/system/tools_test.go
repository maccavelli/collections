package system

import (
	"bytes"
	"context"
	"testing"

	"mcp-server-brainstorm/internal/config"
	"mcp-server-brainstorm/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetInternalLogsTool_Handle(t *testing.T) {
	buffer := &LogBuffer{}
	buffer.Write([]byte("line1\nline2\nline3\n"))
	tool := &GetInternalLogsTool{Buffer: buffer}

	ctx := context.Background()
	input := LogsInput{MaxLines: 2}

	// Test Handle
	res, _, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Errorf("Handle failed: %v", err)
	}
	txt := res.Content[0].(*mcp.TextContent).Text
	if txt != "line2\nline3" {
		t.Errorf("expected last 2 lines, got %q", txt)
	}

	// Verify Name
	if tool.Name() != "get_internal_logs" {
		t.Errorf("expected get_internal_logs, got %s", tool.Name())
	}
}

func TestLogBuffer_Trim(t *testing.T) {
	// Simple test to ensure write/string works.
	lb := &LogBuffer{}
	lb.Write([]byte("hello"))
	if lb.String() != "hello" {
		t.Errorf("got %q, want hello", lb.String())
	}
}

func TestGetInternalLogsTool_Register(t *testing.T) {
	buffer := &LogBuffer{}
	Register(buffer)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, &mcp.ServerOptions{})
	tool := &GetInternalLogsTool{Buffer: buffer}
	tool.Register(&util.MockSessionProvider{Srv: srv})
}

func TestLogBuffer_TrimLarge(t *testing.T) {
	lb := &LogBuffer{}
	largeData := bytes.Repeat([]byte("a\n"), config.LogBufferLimit+100)
	lb.Write(largeData)
	if len(lb.String()) > config.LogBufferLimit {
		t.Errorf("buffer should be trimmed")
	}
}
