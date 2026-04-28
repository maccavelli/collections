package client

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"testing"
)

func TestNewMCPClient(t *testing.T) {
	c := NewMCPClient("http://localhost:1234")
	if c == nil {
		t.Error("client nil")
	}
	if c.RecallEnabled() {
		t.Error("should be false by default until connected")
	}
}

func TestClientStart_Invalid(t *testing.T) {
	c := NewMCPClient("http://invalid-url-that-does-not-exist:9999")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // instantly cancel to hit error branches

	// Should return/exit cleanly without panicking and remain disabled
	c.Start(ctx)

	if c.RecallEnabled() {
		t.Error("expected false")
	}
}

func TestClientCall_WithoutStart(t *testing.T) {
	c := NewMCPClient("http://localhost:1234")
	_, err := c.CallDatabaseTool(context.Background(), "test", nil)
	if err == nil {
		t.Error("expected error calling before start")
	}
}

func TestClient_ExtractErrorText(t *testing.T) {
	res := &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "test error"}},
	}
	if extractErrorText(res) != "test error" {
		t.Error("expected test error")
	}

	resEmpty := &mcp.CallToolResult{}
	if extractErrorText(resEmpty) != "unknown error" {
		t.Error("expected unknown error")
	}
}
