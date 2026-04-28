package handler

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResolveURN_Invalid(t *testing.T) {
	ps := &ProxyService{Handler: &OrchestratorHandler{}}
	_, _, _, _, err := ps.ResolveURN(context.Background(), "nocolon")
	if err == nil {
		t.Fatal("expected error for URN without colon")
	}
}

func TestResolveURN_MagicToolsPassthrough(t *testing.T) {
	ps := &ProxyService{Handler: &OrchestratorHandler{}}
	server, tool, urn, record, err := ps.ResolveURN(context.Background(), "magictools:search_tools")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server != "magictools" {
		t.Errorf("expected server 'magictools', got %q", server)
	}
	if tool != "search_tools" {
		t.Errorf("expected tool 'search_tools', got %q", tool)
	}
	if urn != "magictools:search_tools" {
		t.Errorf("expected urn 'magictools:search_tools', got %q", urn)
	}
	if record != nil {
		t.Errorf("expected nil record for magictools passthrough, got %v", record)
	}
}

func TestResolveURN_ThreePartURN(t *testing.T) {
	ps := &ProxyService{Handler: &OrchestratorHandler{}}
	server, tool, urn, _, err := ps.ResolveURN(context.Background(), "magictools:category:tool_name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server != "magictools" {
		t.Errorf("expected server 'magictools', got %q", server)
	}
	if tool != "tool_name" {
		t.Errorf("expected tool 'tool_name', got %q", tool)
	}
	if urn != "magictools:tool_name" {
		t.Errorf("expected urn 'magictools:tool_name', got %q", urn)
	}
}

func TestResolveURN_PrefixStripping(t *testing.T) {
	ps := &ProxyService{Handler: &OrchestratorHandler{}}
	server, tool, _, _, err := ps.ResolveURN(context.Background(), "urn:magictools:search_tools")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server != "magictools" {
		t.Errorf("expected server 'magictools', got %q", server)
	}
	if tool != "search_tools" {
		t.Errorf("expected tool 'search_tools', got %q", tool)
	}
}

func TestEnsureServerReady(t *testing.T) {
	h, store, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)
	defer store.Close()

	ps := NewProxyService(h)
	// Server not in config
	latency := ps.EnsureServerReady(context.Background(), "unknown")
	if latency < 0 {
		t.Error("latency should be non-negative")
	}

	// Server in config (will fail to connect but should hit the code path)
	latency = ps.EnsureServerReady(context.Background(), "test-server")
	if latency < 0 {
		t.Error("latency should be non-negative")
	}
}

func TestMinifyResponse(t *testing.T) {
	h, store, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)
	defer store.Close()

	ps := NewProxyService(h)
	res := &mcp.CallToolResult{
		StructuredContent: map[string]any{
			"status": "success",
			"data":   "value",
			"null":   nil,
		},
	}

	minified := ps.MinifyResponse(context.Background(), res, "test_server", "test_tool", 0, 2000)
	if minified == nil {
		t.Fatal("expected non-nil result")
	}
	if len(minified.Content) == 0 {
		t.Fatal("expected content to be added")
	}
	text := minified.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "### Summary") {
		t.Errorf("expected hybrid markdown, got: %s", text)
	}
}

func TestValidateArguments_SchemaCache(t *testing.T) {
	h, store, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)
	defer store.Close()

	ps := NewProxyService(h)

	// ValidateArguments with nil record and non-existent tool should return nil (no error)
	err := ps.ValidateArguments(context.Background(), "nonexistent:tool", nil, map[string]any{})
	if err != nil {
		t.Errorf("expected nil error for non-existent tool, got: %v", err)
	}
}

func TestInspectResponse(t *testing.T) {
	ps := &ProxyService{Handler: &OrchestratorHandler{}}
	ctx := context.Background()

	t.Run("NilResponse", func(t *testing.T) {
		diag := ps.InspectResponse(ctx, nil, "test", "tool")
		if diag != nil {
			t.Errorf("expected nil for nil response, got: %+v", diag)
		}
	})

	t.Run("ErrorResponse", func(t *testing.T) {
		res := &mcp.CallToolResult{IsError: true}
		diag := ps.InspectResponse(ctx, res, "test", "tool")
		if diag != nil {
			t.Errorf("expected nil for error response, got: %+v", diag)
		}
	})

	t.Run("EmptyResultsArray", func(t *testing.T) {
		res := &mcp.CallToolResult{
			StructuredContent: map[string]any{
				"results": []any{},
				"status":  "ok",
			},
		}
		diag := ps.InspectResponse(ctx, res, "ddg-search", "search_web")
		if diag == nil || !diag.Detected {
			t.Fatal("expected soft failure for empty results array")
		}
		if diag.Reason != "results array is empty" {
			t.Errorf("unexpected reason: %s", diag.Reason)
		}
	})

	t.Run("ZeroTotalCountInMetadata", func(t *testing.T) {
		res := &mcp.CallToolResult{
			StructuredContent: map[string]any{
				"data": map[string]any{
					"metadata": map[string]any{
						"total_count": float64(0),
					},
					"type": "web",
				},
			},
		}
		diag := ps.InspectResponse(ctx, res, "ddg-search", "search_web")
		if diag == nil || !diag.Detected {
			t.Fatal("expected soft failure for zero total_count")
		}
		if diag.Reason != "metadata.total_count is 0" {
			t.Errorf("unexpected reason: %s", diag.Reason)
		}
	})

	t.Run("NormalResponse", func(t *testing.T) {
		res := &mcp.CallToolResult{
			StructuredContent: map[string]any{
				"results": []any{"item1", "item2"},
				"count":   float64(2),
			},
		}
		diag := ps.InspectResponse(ctx, res, "test", "tool")
		if diag != nil {
			t.Errorf("expected nil for normal response, got: %+v", diag)
		}
	})

	t.Run("TextContentJSONFallback", func(t *testing.T) {
		res := &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: `{"results": [], "type": "web"}`},
			},
		}
		diag := ps.InspectResponse(ctx, res, "test", "tool")
		if diag == nil || !diag.Detected {
			t.Fatal("expected soft failure from text content JSON")
		}
	})
}

func TestExecuteProxy(t *testing.T) {
	h, store, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)
	defer store.Close()

	ps := NewProxyService(h)

	// Test executing a proxy with an unknown server. Should fail at connect/ping.
	args := map[string]any{"key": "value"}

	res, err := ps.ExecuteProxy(context.Background(), "unknown-server", "some-tool", args, 30*time.Second)
	if err == nil {
		if !res.IsError {
			t.Errorf("Expected an error or IsError=true when connecting to unknown server")
		}
	}
}
