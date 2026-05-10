package server

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-recall/internal/config"
	"mcp-server-recall/internal/memory"
)

func buildReq(argJSON string) *mcp.CallToolRequest {
	return &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "test",
			Arguments: json.RawMessage(argJSON),
		},
	}
}

func TestHandleGetStandard_NotFound(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "standards-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := memory.NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	defer store.Close()

	rs := &MCPRecallServer{store: store}
	req := buildReq(`{"key": "non-existent-key"}`)

	res, _, err := rs.handleGetStandard(context.Background(), req, GetStandardInput{Key: "non-existent-key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.IsError {
		t.Errorf("expected IsError=true for non-existent key")
	}
}

func TestHandleDeleteStandards_NoArgs(t *testing.T) {
	rs := &MCPRecallServer{}
	req := buildReq(`{}`)
	res, _, err := rs.handleDeleteStandards(context.Background(), req, DeleteStandardsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.IsError {
		t.Errorf("expected IsError=true for empty args")
	}
}
