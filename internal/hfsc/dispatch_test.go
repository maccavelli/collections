package hfsc

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sync"
	"time"
)

type MockLogSession struct {
	Logs []string
	Mu   sync.Mutex
	Done chan struct{}
}

func (m *MockLogSession) Log(ctx context.Context, params *mcp.LoggingMessageParams) error {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	var dataStr string
	if b, ok := params.Data.(json.RawMessage); ok {
		dataStr = string(b)
	} else if s, ok := params.Data.(string); ok {
		dataStr = s
	}
	m.Logs = append(m.Logs, dataStr)
	if strings.Contains(dataStr, "HFSC_FINALIZE") {
		select {
		case <-m.Done:
		default:
			close(m.Done)
		}
	}
	return nil
}

func TestStreamHeavyPayload_Orchestrated(t *testing.T) {
	os.Setenv("MCP_ORCHESTRATOR_OWNED", "true")
	defer os.Unsetenv("MCP_ORCHESTRATOR_OWNED")

	session := &MockLogSession{Done: make(chan struct{})}
	content := "some heavy data"
	reader := strings.NewReader(content)

	res, err := StreamHeavyPayload(context.Background(), session, "test.json", "proj1", "model1", reader)
	if err != nil {
		t.Fatalf("StreamHeavyPayload failed: %v", err)
	}

	if res.Meta["hfsc_stream"] != true {
		t.Errorf("expected hfsc_stream meta to be true")
	}

	// Wait for stream to finish
	select {
	case <-session.Done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream finalizer")
	}

	session.Mu.Lock()
	defer session.Mu.Unlock()
	if len(session.Logs) < 2 { // At least one chunk + finalizer
		t.Errorf("expected at least 2 log messages, got %d", len(session.Logs))
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()
	if len(id1) != 32 {
		t.Errorf("expected session ID length 32, got %d", len(id1))
	}
	if id1 == id2 {
		t.Error("expected session IDs to be unique")
	}
}

func TestStreamHeavyPayload_Standalone(t *testing.T) {
	os.Setenv("MCP_ORCHESTRATOR_OWNED", "false")
	defer os.Unsetenv("MCP_ORCHESTRATOR_OWNED")

	ctx := context.Background()
	filename := "test.txt"
	projectID := "proj-123"
	model := "gpt-4"
	payload := "this is a massive payload for standalone mode"
	reader := strings.NewReader(payload)

	result, err := StreamHeavyPayload(ctx, nil, filename, projectID, model, reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be non-nil")
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	if textContent.Text != payload {
		t.Errorf("expected text %q, got %q", payload, textContent.Text)
	}
}

type errorReader struct{}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, os.ErrPermission
}

func TestStreamHeavyPayload_StandaloneError(t *testing.T) {
	os.Setenv("MCP_ORCHESTRATOR_OWNED", "false")
	defer os.Unsetenv("MCP_ORCHESTRATOR_OWNED")

	ctx := context.Background()
	_, err := StreamHeavyPayload(ctx, nil, "test.txt", "proj", "model", &errorReader{})
	if err == nil {
		t.Fatal("expected error from failed read, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read stream") {
		t.Errorf("unexpected error message: %v", err)
	}
}

type ErrorLogSession struct {
	Done chan struct{}
}

func (m *ErrorLogSession) Log(ctx context.Context, params *mcp.LoggingMessageParams) error {
	select {
	case <-m.Done:
	default:
		close(m.Done)
	}
	return os.ErrPermission
}

func TestStreamHeavyPayload_LogError(t *testing.T) {
	os.Setenv("MCP_ORCHESTRATOR_OWNED", "true")
	defer os.Unsetenv("MCP_ORCHESTRATOR_OWNED")

	session := &ErrorLogSession{Done: make(chan struct{})}
	reader := strings.NewReader("data")

	_, _ = StreamHeavyPayload(context.Background(), session, "test.json", "proj", "model", reader)

	select {
	case <-session.Done:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out")
	}
}

func TestStreamHeavyPayload_ReadErrorOrchestrated(t *testing.T) {
	os.Setenv("MCP_ORCHESTRATOR_OWNED", "true")
	defer os.Unsetenv("MCP_ORCHESTRATOR_OWNED")

	session := &MockLogSession{Done: make(chan struct{})}
	_, _ = StreamHeavyPayload(context.Background(), session, "test.json", "proj", "model", &errorReader{})

	select {
	case <-session.Done:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out")
	}
}

func TestGenerateSessionID_Fallback(t *testing.T) {
	// Simple coverage for generateSessionID
	id := generateSessionID()
	if len(id) != 32 {
		t.Errorf("expected length 32, got %d", len(id))
	}
}
