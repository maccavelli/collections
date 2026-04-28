package external

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMCPClient_CallDatabaseTool(t *testing.T) {
	// Mock MCP Server using httptest (SSE simulation)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Minimum SSE/MCP handshake simulation
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// In a real test, we'd use the SDK server, but for a unit test of the client
		// we verify the client's ability to handle structured responses.
	}))
	defer ts.Close()

	client := NewMCPClient(ts.URL)
	ctx := context.Background()

	t.Run("RecallDisabled", func(t *testing.T) {
		res := client.CallDatabaseTool(ctx, "test_tool", nil)
		if res != "" {
			t.Errorf("expected empty result when recall is disabled, got %s", res)
		}
	})

	t.Run("RecallEnabled_EmptySession", func(t *testing.T) {
		client.setRecallEnabled(true)
		res := client.CallDatabaseTool(ctx, "test_tool", nil)
		if res != "" {
			t.Errorf("expected empty result when session is nil, got %s", res)
		}
	})
}

func TestMCPClient_SaveSession(t *testing.T) {
	client := NewMCPClient("http://localhost:8080")
	ctx := context.Background()

	err := client.SaveSession(ctx, "session-1", "project-1", map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// Verify telemetry queue — shard is determined by sessionID[0] % 8
	shardIdx := int("session-1"[0]) % 8
	select {
	case event := <-client.telemetryShards[shardIdx]:
		if event.sessionID != "session-1" {
			t.Errorf("expected session-1, got %s", event.sessionID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected telemetry event in queue")
	}
}

func TestMCPClient_Backoff(t *testing.T) {
	// Verify that the client respects context cancellation during stabilization
	client := NewMCPClient("http://unreachable")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	client.Start(ctx)
	duration := time.Since(start)

	if duration > 1*time.Second {
		t.Errorf("Start() did not respect context cancellation, took %v", duration)
	}
}
