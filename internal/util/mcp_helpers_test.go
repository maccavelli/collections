package util

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNopReadCloser(t *testing.T) {
	nrc := NopReadCloser{Reader: bytes.NewReader([]byte("test"))}
	if err := nrc.Close(); err != nil {
		t.Errorf("expected no error from Close, got %v", err)
	}
}

func TestNopWriteCloser(t *testing.T) {
	var buf bytes.Buffer
	nwc := NopWriteCloser{Writer: &buf}
	if err := nwc.Close(); err != nil {
		t.Errorf("expected no error from Close, got %v", err)
	}
}

// For HardenedAddTool, since we can't easily invoke the internal handler without a full real client,
// at least invoke Registration.
func TestHardenedAddTool(t *testing.T) {
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "1.0"},
		&mcp.ServerOptions{},
	)
	tool := &mcp.Tool{}
	json.Unmarshal([]byte(`{"name":"test_tool","inputSchema":{"type":"object","properties":{}}}`), tool)

	handler := func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		// Just returning safely to avoid panic on test load
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "success"},
			},
		}, nil, nil
	}

	HardenedAddTool(&MockSessionProvider{Srv: srv}, tool, handler)
}
