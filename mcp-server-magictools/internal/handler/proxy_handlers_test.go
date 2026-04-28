package handler

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProxyHandlersMethods(t *testing.T) {
	h, store, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)
	defer store.Close()

	ctx := context.Background()

	t.Run("AlignTools", func(t *testing.T) {
		args, _ := json.Marshal(map[string]string{"query": "sync"})
		req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: args}}
		res, err := h.AlignTools(ctx, req)
		if err != nil {
			t.Errorf("AlignTools failed: %v", err)
		}
		if res == nil || len(res.Content) == 0 {
			t.Fatal("expected result content")
		}
		text := res.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, "sync_ecosystem") {
			t.Errorf("expected text to contain sync_ecosystem, got: %s", text)
		}
	})

	t.Run("CallProxy", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"urn": "unknown:urn"})
		req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: args}}
		res, err := h.CallProxy(ctx, req)
		// We expect this to fail gracefully or return a formatted error within the proxy service
		if err == nil {
			if res.IsError == false {
				t.Errorf("Expected CallProxy to fail or return an error flag for unknown URN")
			}
		}
	})

	t.Run("measureResponseSize", func(t *testing.T) {
		r := &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "12345"},
			},
			StructuredContent: map[string]any{"key": "val"},
		}
		s := measureResponseSize(r)
		if s == 0 {
			t.Errorf("expected size > 0")
		}
	})

	t.Run("isPreferred", func(t *testing.T) {
		_ = h.isPreferred(ctx, "magictools:sync", "sync")
	})
}
