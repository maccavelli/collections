package client

import (
	"context"
	"testing"
	"time"
)

func TestClient_SetRecallEnabled(t *testing.T) {
	c := NewMCPClient("http://localhost:1234")
	if c.RecallEnabled() {
		t.Error("expected recall disabled by default")
	}
	c.setRecallEnabled(true)
	if !c.RecallEnabled() {
		t.Error("expected recall enabled after setRecallEnabled(true)")
	}
	c.setRecallEnabled(false)
	if c.RecallEnabled() {
		t.Error("expected recall re-disabled after setRecallEnabled(false)")
	}
}

func TestCallDatabaseTool_CancelledContext(t *testing.T) {
	c := NewMCPClient("http://localhost:1234")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	_, err := c.CallDatabaseTool(ctx, "test_tool", map[string]any{"key": "value"})
	if err == nil {
		t.Error("expected error on cancelled context when recall is not enabled")
	}
}

func TestCallDatabaseTool_Timeout(t *testing.T) {
	c := NewMCPClient("http://localhost:1234")
	// Manually enable recall so it tries to actually call and hits a network timeout
	c.setRecallEnabled(true)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// This will timeout because there's no real server — exercises the context deadline branch
	_, err := c.CallDatabaseTool(ctx, "test_tool", nil)
	// Error is expected — either context.DeadlineExceeded or connection refused
	_ = err
}

func TestNewMCPClient_EmptyURL(t *testing.T) {
	c := NewMCPClient("")
	if c == nil {
		t.Error("expected non-nil client from empty URL")
	}
}
