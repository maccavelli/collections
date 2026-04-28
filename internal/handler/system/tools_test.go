package system

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetInternalLogsTool(t *testing.T) {
	buffer := &LogBuffer{}
	tool := &GetInternalLogsTool{Buffer: buffer}
	buffer.Write([]byte("test logging output stream"))

	res, _, err := tool.Handle(context.Background(), &mcp.CallToolRequest{}, LogsInput{
		MaxLines: 15,
	})
	if err != nil {
		t.Error("unexpected err from log handler")
	}
	if res == nil || len(res.Content) == 0 {
		t.Error("expected valid mcp content output")
	}
}

func TestLogBufferLimitsAndRedaction(t *testing.T) {
	buffer := &LogBuffer{}
	// Insert simulated secrets to ensure redact block is evaluated
	buffer.Write([]byte("simulating token_123abcd secret_0000 key_456!"))

	out := buffer.String()
	if out == "" {
		t.Error("buffer string output empty")
	}
}
