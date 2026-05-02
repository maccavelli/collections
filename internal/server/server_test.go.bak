package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-recall/internal/config"
	"mcp-server-recall/internal/memory"
	"mcp-server-recall/internal/search"
)

type nopCloser struct {
	*bytes.Buffer
}

func (nopCloser) Close() error { return nil }

func TestMCPServer(t *testing.T) {
	logger := slog.Default()
	cfg := config.New("1.0")
	srv, _ := NewMCPRecallServer(cfg, nil, nil, logger)
	if srv == nil {
		t.Fatal("Server returned nil")
	}

	stdout := nopCloser{Buffer: &bytes.Buffer{}}
	reader := nopCloser{Buffer: bytes.NewBufferString("{}")}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()

	err := srv.Serve(ctx, stdout, reader)
	if err != nil && err != context.DeadlineExceeded && err.Error() != "context deadline exceeded" {
	}
}

func createTestServer(t *testing.T) (*MCPRecallServer, *memory.MemoryStore, func()) {
	logger := slog.Default()
	os.Setenv("MCP_RECALL_EXPORTDIR", t.TempDir())
	cfg := config.New("1.0")
	store, _ := memory.NewMemoryStore(context.Background(), t.TempDir(), "", 0, config.New("test").BatchSettings())
	lb := &LogBuffer{}
	lb.Write([]byte("test log 1\ntest log 2\n"))
	srv, _ := NewMCPRecallServer(cfg, store, lb, logger)

	return srv, store, func() { store.Close() }
}

func testJSONError(t *testing.T, handler func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	req := &mcp.CallToolRequest{}
	_ = json.Unmarshal([]byte(`{"params":{"name":"test", "arguments": {}}}`), req)

	// req.Params is a pointer (*mcp.CallToolRequestParams or similar), so if it's nil we must panic safely
	// Actually, json.Unmarshal should allocate req.Params because the JSON has "params": {...}

	// Create another request that just forces the server string parsing to fail by passing raw bytes
	req2 := &mcp.CallToolRequest{}
	_ = json.Unmarshal([]byte(`{"params":{"name":"test"}}`), req2)
	// If req2.Params is nil due to Go SDK typing, just use the unmarshal trick directly
	req3 := &mcp.CallToolRequest{}
	_ = json.Unmarshal([]byte(`{"params":{"name":"test", "arguments": 123}}`), req3)
	// Passing an integer as arguments will cause the handler to fail its json.Unmarshal(req.Params.Arguments)

	res, err := handler(context.Background(), req3)
	if err == nil {
		t.Errorf("Expected JSON unmarshal error, got nil")
	}
	if res != nil {
		t.Errorf("Expected nil result on unmarshal error")
	}
}

func makeReq(args string) *mcp.CallToolRequest {
	req := &mcp.CallToolRequest{}
	jsonStr := fmt.Sprintf(`{"params":{"name":"test", "arguments": %s}}`, args)
	_ = json.Unmarshal([]byte(jsonStr), req)
	return req
}

func TestHandlers_Remember_And_Recall(t *testing.T) {
	srv, _, cleanup := createTestServer(t)
	defer cleanup()
	ctx := context.Background()

	// JSON Error
	testJSONError(t, srv.handleRemember)
	testJSONError(t, srv.handleRecall)

	// Remember Success
	res, err := srv.handleRemember(ctx, makeReq(`{"key":"k1","value":"v1","category":"cat1","tags":["t1"]}`))
	if err != nil || res.IsError {
		t.Errorf("Remember failed: %v", err)
	}

	// Recall Success
	res, err = srv.handleRecall(ctx, makeReq(`{"key":"k1"}`))
	if err != nil || res.IsError {
		t.Errorf("Recall failed: %v", err)
	}

	// Recall Missing Key (Store Error)
	res, _ = srv.handleRecall(ctx, makeReq(`{"key":"missing"}`))
	if !res.IsError {
		t.Errorf("Recall expected store error")
	}
}

func TestHandlers_Search_And_Stats(t *testing.T) {
	srv, _, cleanup := createTestServer(t)
	defer cleanup()
	ctx := context.Background()

	testJSONError(t, srv.handleSearch)

	// Seed
	srv.handleRemember(ctx, makeReq(`{"key":"s1","value":"search me","category":"c1"}`))

	// Search
	res, err := srv.handleSearch(ctx, makeReq(`{"query":"search","limit":0}`)) // tests limit default
	if err != nil || res.IsError {
		t.Errorf("Search failed: %v", err)
	}

	// Metrics
	res, err = srv.handleGetMetrics(ctx, makeReq(`{}`))
	if err != nil || res.IsError {
		t.Errorf("Metrics failed: %v", err)
	}
}

func TestHandlers_Others(t *testing.T) {
	srv, _, cleanup := createTestServer(t)
	ctx := context.Background()

	testJSONError(t, srv.handleRecallRecent)
	testJSONError(t, srv.handleForget)
	testJSONError(t, srv.handleGetLogs)

	// RecallRecent
	res, _ := srv.handleRecallRecent(ctx, makeReq(`{"count": 0}`))
	if res.IsError {
		t.Errorf("RecallRecent failed")
	}

	// List
	res, _ = srv.handleList(ctx, makeReq(`{}`))
	if res.IsError {
		t.Errorf("List failed")
	}

	// ListCategories
	res, _ = srv.handleListCategories(ctx, makeReq(`{}`))
	if res.IsError {
		t.Errorf("ListCategories failed")
	}

	// GetLogs
	res, _ = srv.handleGetLogs(ctx, makeReq(`{"max_lines": 0}`))
	if res.IsError {
		t.Errorf("GetLogs failed")
	}

	// Forget (force error state by closing store manually)
	cleanup() // This closes the store
	res, _ = srv.handleForget(ctx, makeReq(`{"key":"missing"}`))
	if !res.IsError {
		t.Errorf("Expected IsError when store is closed")
	}

	res, _ = srv.handleRecallRecent(ctx, makeReq(`{"count": 5}`))
	if !res.IsError {
		t.Errorf("Expected IsError when store is closed")
	}
}

func TestHandlers_Export_And_Import(t *testing.T) {
	srv, _, cleanup := createTestServer(t)
	defer cleanup()
	ctx := context.Background()

	// Test malformed JSON parameters failing cleanly
	testJSONError(t, srv.handleExportMemories)
	testJSONError(t, srv.handleImportMemories)

	// Test Export with valid missing filename (should auto-generate and succeed)
	res, err := srv.handleExportMemories(ctx, makeReq(`{}`))
	if err != nil || res.IsError {
		t.Errorf("ExportMemories with defaulted filename failed: %v", err)
	}

	// Test Export with valid filename
	res, err = srv.handleExportMemories(ctx, makeReq(`{"filename": "test-export.jsonl"}`))
	if err != nil || res.IsError {
		t.Errorf("ExportMemories failed: %v", err)
	}

	// Test Import without filename (should gracefully fail as file is required to exist)
	res, _ = srv.handleImportMemories(ctx, makeReq(`{}`))
	if !res.IsError {
		t.Errorf("ImportMemories with no filename should gracefully fail and map an Error to the client")
	}

	// Test Import with filename
	res, err = srv.handleImportMemories(ctx, makeReq(`{"filename": "test-export.jsonl"}`))
	if err != nil || res.IsError {
		t.Errorf("ImportMemories failed: %v", err)
	}
}

func TestHandlers_ReloadCache(t *testing.T) {
	srv, store, cleanup := createTestServer(t)
	defer cleanup()

	// Initialize search engine for the test
	engine, err := search.InitStorage(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create search engine: %v", err)
	}
	ctx := context.Background()
	if err := store.SetSearchEngine(ctx, engine); err != nil {
		t.Fatalf("Failed to set search engine: %v", err)
	}

	res, err := srv.handleReloadCache(ctx, &mcp.CallToolRequest{})
	if err != nil || res.IsError {
		t.Errorf("ReloadCache failed: %v", res.Content)
	}

	if !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "successfully re-synchronized") {
		t.Errorf("Unexpected response: %v", res.Content[0].(*mcp.TextContent).Text)
	}
}
