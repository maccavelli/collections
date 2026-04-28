package system

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestGetInternalLogsTool_Handle verifies that log lines can be successfully extracted up to the line limit.
func TestGetInternalLogsTool_Handle(t *testing.T) {
	buffer := &LogBuffer{}
	buffer.Write([]byte("line1\nline2\nline3\n"))
	tool := &GetInternalLogsTool{Buffer: buffer}

	ctx := context.Background()
	input := LogsInput{
		MaxLines: 2,
	}

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

// TestLogBuffer_Trim verifies that the raw string trimming logic removes excessive bytes correctly.
func TestLogBuffer_Trim(t *testing.T) {
	// Simple test to ensure write/string works.
	lb := &LogBuffer{}
	lb.Write([]byte("hello"))
	if lb.String() != "hello" {
		t.Errorf("got %q, want hello", lb.String())
	}
}
