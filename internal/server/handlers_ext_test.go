package server

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-recall/internal/config"
	"mcp-server-recall/internal/memory"
)

func TestHandleIngestFiles_InvalidJSON(t *testing.T) {
	rs := &MCPRecallServer{}
	req := buildReq(`{invalid}`)
	_, err := rs.handleIngestFiles(context.Background(), req)
	if err == nil {
		t.Errorf("expected err on invalid json")
	}
}

func TestHandleDeleteMemories_InvalidJSON(t *testing.T) {
	rs := &MCPRecallServer{}
	req := buildReq(`{invalid}`)
	_, err := rs.handleDeleteMemories(context.Background(), req)
	if err == nil {
		t.Errorf("expected err")
	}
}

func TestHandleBatchRemember_Success(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "ext-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := memory.NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	defer store.Close()

	rs := &MCPRecallServer{store: store}

	req := buildReq(`{"entries": [{"key":"test","value":"test","category":"test"}]}`)
	res, err := rs.handleBatchRemember(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error in result")
	}
}

func TestHandleBatchRecall_Empty(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "ext-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := memory.NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	defer store.Close()

	rs := &MCPRecallServer{store: store}

	req := buildReq(`{"keys": ["non-existent"]}`)
	res, err := rs.handleBatchRecall(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// should not throw error if keys are not found, just return empty list or skipped
	if res.IsError {
		t.Errorf("unexpected error in result")
	}
}

func TestHandleSessions_InvalidJSON(t *testing.T) {
	rs := &MCPRecallServer{}

	handlers := []func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error){
		rs.handleSaveSessions,
		rs.handleGetSessions,
		rs.handleListSessions,
	}

	for _, h := range handlers {
		req := buildReq(`{invalid`)
		_, err := h(context.Background(), req)
		if err == nil {
			t.Errorf("expected err for session handler")
		}
	}
}

func TestHandleListSessions_Empty(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "ext-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := memory.NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	defer store.Close()

	rs := &MCPRecallServer{store: store}

	req := buildReq(`{"limit": 10}`)
	res, err := rs.handleListSessions(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error in result")
	}
}

func TestHandleSearchStandards_Empty(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "ext-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := memory.NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	defer store.Close()

	rs := &MCPRecallServer{store: store}

	req := buildReq(`{"query": "auth", "limit": 10}`)
	res, err := rs.handleSearchStandards(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error in result")
	}
}

func TestHandleList_Success(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "ext-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := memory.NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	defer store.Close()

	rs := &MCPRecallServer{store: store}
	req := buildReq(`{}`)
	res, err := rs.handleList(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.IsError {
		t.Errorf("expected clean handleList result")
	}
}

func TestHandleForget_Empty(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "ext-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := memory.NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	defer store.Close()

	rs := &MCPRecallServer{store: store}
	req := buildReq(`{"key": "non-existent"}`)
	res, err := rs.handleForget(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error deleting missing key")
	}
}

func TestHandleExportMemories_Empty(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "export-test-*")
	defer os.RemoveAll(tmpDir)

	c := config.New("test")
	store, _ := memory.NewMemoryStore(context.Background(), tmpDir, "", 0, c.BatchSettings())
	defer store.Close()

	// Use a unique filename to avoid O_EXCL collision with stale files from prior runs.
	exportFile := fmt.Sprintf("test-empty-%d.jsonl", time.Now().UnixNano())

	rs := &MCPRecallServer{store: store, cfg: c}
	req := buildReq(fmt.Sprintf(`{"filename": %q}`, exportFile))
	res, err := rs.handleExportMemories(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.IsError {
		// Extract error message for diagnosis.
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				t.Errorf("unexpected error during export: %s", tc.Text)
			}
		}
	}
}
