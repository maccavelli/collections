package util

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHardenedAddTool(t *testing.T) {
	handler := func(ctx context.Context, req *mcp.CallToolRequest, input any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "success"},
			},
		}, nil, nil
	}

	InternalWrapHandler[any, any](&mcp.Tool{Name: "test"}, handler)
}

func TestSafeToolHandler_PanicRecovery(t *testing.T) {
	handler := func(ctx context.Context, req *mcp.CallToolRequest, input any) (*mcp.CallToolResult, any, error) {
		panic("simulated panic")
	}
	safeHandler := InternalWrapHandler[any, any](&mcp.Tool{Name: "test"}, handler)
	res, _, err := safeHandler(context.Background(), &mcp.CallToolRequest{}, nil)
	if err != nil {
		t.Errorf("expected no error (suppressed by handler), got %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true from panic recovery")
	}
}

func TestSafeToolHandler_JSONMandate(t *testing.T) {
	handler := func(ctx context.Context, req *mcp.CallToolRequest, input any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			StructuredContent: map[string]interface{}{"key": "value"},
		}, nil, nil
	}
	safeHandler := InternalWrapHandler[any, any](&mcp.Tool{Name: "test"}, handler)
	res, _, _ := safeHandler(context.Background(), &mcp.CallToolRequest{}, nil)
	if res.Content == nil {
		t.Errorf("expected Content to be initialized as empty slice to satisfy JSON mandate")
	}
}

func TestSafeToolHandler_AuditLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	defer slog.SetDefault(slog.Default())

	handler := func(ctx context.Context, req *mcp.CallToolRequest, input any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, nil, nil
	}
	safeHandler := InternalWrapHandler[any, any](&mcp.Tool{Name: "test"}, handler)
	_, _, _ = safeHandler(context.Background(), &mcp.CallToolRequest{}, nil)

	output := buf.String()
	if !strings.Contains(output, `"msg":"audit"`) {
		t.Errorf("expected audit log line, got: %s", output)
	}
	if !strings.Contains(output, `"ok":true`) {
		t.Errorf("expected ok=true in audit log, got: %s", output)
	}
	if !strings.Contains(output, `"client":"stdio"`) {
		t.Errorf("expected client=stdio (default), got: %s", output)
	}
}

func TestSafeToolHandler_AuditLogWithClient(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	defer slog.SetDefault(slog.Default())

	handler := func(ctx context.Context, req *mcp.CallToolRequest, input any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, nil, nil
	}

	ctx := WithClient(context.Background(), "mcp-server-brainstorm")
	safeHandler := InternalWrapHandler[any, any](&mcp.Tool{Name: "test"}, handler)
	_, _, _ = safeHandler(ctx, &mcp.CallToolRequest{}, nil)

	output := buf.String()
	if !strings.Contains(output, `"client":"mcp-server-brainstorm"`) {
		t.Errorf("expected client=mcp-server-brainstorm, got: %s", output)
	}
}

func TestSafeToolHandler_AuditLogError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	defer slog.SetDefault(slog.Default())

	handler := func(ctx context.Context, req *mcp.CallToolRequest, input any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "failed"}},
		}, nil, nil
	}
	safeHandler := InternalWrapHandler[any, any](&mcp.Tool{Name: "test"}, handler)
	_, _, _ = safeHandler(context.Background(), &mcp.CallToolRequest{}, nil)

	output := buf.String()
	if !strings.Contains(output, `"ok":false`) {
		t.Errorf("expected ok=false in audit log, got: %s", output)
	}
}

func TestWithClient_RoundTrip(t *testing.T) {
	ctx := context.Background()

	// Default should be "stdio"
	if got := ClientFromContext(ctx); got != "stdio" {
		t.Errorf("expected 'stdio', got %q", got)
	}

	// Set and retrieve
	ctx = WithClient(ctx, "test-client")
	if got := ClientFromContext(ctx); got != "test-client" {
		t.Errorf("expected 'test-client', got %q", got)
	}

	// Empty string should fall back to "stdio"
	ctx2 := WithClient(context.Background(), "")
	if got := ClientFromContext(ctx2); got != "stdio" {
		t.Errorf("expected 'stdio' for empty string, got %q", got)
	}
}

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

func TestHardenedAddTool_RealServer(t *testing.T) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, &mcp.ServerOptions{})
	tool := mcp.Tool{Name: "test_tool"}

	handler := func(ctx context.Context, req *mcp.CallToolRequest, input any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "success"}}}, nil, nil
	}

	HardenedAddTool(srv, &tool, handler)

}
