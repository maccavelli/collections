package external

import (
	"context"
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

	// Should exit cleanly without panic
	c.Start(ctx)

	if c.RecallEnabled() {
		t.Error("expected false")
	}
}

func TestClientCall_WithoutStart(t *testing.T) {
	c := NewMCPClient("http://localhost:1234")
	res := c.CallDatabaseTool(context.Background(), "test", nil)
	if res != "" {
		t.Error("expected empty string calling before start")
	}

	// SaveSession enqueues to the telemetry channel (buffered, cap 1000).
	// It only errors when the channel is full. With recall disabled, the
	// backoffFlusher silently drops via the circuit breaker in CallDatabaseTool.
	err := c.SaveSession(context.Background(), "1", "global", nil)
	if err != nil {
		t.Errorf("expected nil error from SaveSession enqueue, got: %v", err)
	}

	_, err = c.GetSession(context.Background(), "test-session-1")
	if err == nil {
		t.Error("expected err from GetSession when disabled")
	}
}
