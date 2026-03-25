package server

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-recall/internal/memory"
)

func TestServerHandlers(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "recall-server-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := memory.NewMemoryStore(tmpDir)
	defer store.Close()

	srv, _ := NewMCPRecallServer("test", "v1", store)

	ctx := context.Background()

	// Test Remember
	args := json.RawMessage(`{"key": "k1", "value": "v1", "tags": ["t1"]}`)
	res, err := srv.handleRemember(ctx, &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "remember",
			Arguments: args,
		},
	})
	if err != nil {
		t.Errorf("handleRemember failed: %v", err)
	}
	if res.IsError {
		t.Errorf("handleRemember returned error: %v", res.Content[0].(*mcp.TextContent).Text)
	}

	// Test Recall
	args = json.RawMessage(`{"key": "k1"}`)
	res, err = srv.handleRecall(ctx, &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "recall",
			Arguments: args,
		},
	})
	if err != nil {
		t.Errorf("handleRecall failed: %v", err)
	}

	// Test Search
	args = json.RawMessage(`{"query": "v1"}`)
	res, err = srv.handleSearch(ctx, &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "search_memories",
			Arguments: args,
		},
	})
	if err != nil {
		t.Errorf("handleSearch failed: %v", err)
	}

	// Test Stats
	res, err = srv.handleStats(ctx, &mcp.CallToolRequest{})
	if err != nil {
		t.Errorf("handleStats failed: %v", err)
	}

	// Test Forget
	args = json.RawMessage(`{"key": "k1"}`)
	res, err = srv.handleForget(ctx, &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "forget",
			Arguments: args,
		},
	})
	if err != nil {
		t.Errorf("handleForget failed: %v", err)
	}
}
