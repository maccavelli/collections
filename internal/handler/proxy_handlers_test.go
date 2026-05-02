package handler

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magictools/internal/db"
)

func TestAlignTools(t *testing.T) {
	h, store, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)
	defer store.Close()

	ctx := context.Background()

	// Add some mock tools to the store
	tools := []*db.ToolRecord{
		{
			URN:         "test:tool1",
			Name:        "tool1",
			Server:      "test",
			Description: "searches for something",
			Category:    "search",
		},
		{
			URN:         "test:tool2",
			Name:        "tool2",
			Server:      "test",
			Description: "refactors code",
			Category:    "refactor",
		},
	}
	for _, tool := range tools {
		if err := store.SaveTool(tool); err != nil {
			t.Fatalf("failed to save tool: %v", err)
		}
	}

	// 1. Test basic search
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "align_tools",
			Arguments: json.RawMessage(`{"query": "search"}`),
		},
	}

	res, err := h.AlignTools(ctx, req)
	if err != nil {
		t.Fatalf("AlignTools failed: %v", err)
	}

	if len(res.Content) == 0 {
		t.Fatalf("expected results, got none")
	}

	// Verify content
	found := false
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if strings.Contains(tc.Text, "test:tool1") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected to find test:tool1 in results")
	}

	// 2. Test server filtering
	req = &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "align_tools",
			Arguments: json.RawMessage(`{"query": "refactor", "server_name": "test"}`),
		},
	}

	res, err = h.AlignTools(ctx, req)
	if err != nil {
		t.Fatalf("AlignTools failed: %v", err)
	}

	found = false
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if strings.Contains(tc.Text, "test:tool2") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected to find test:tool2 in filtered results")
	}
}
