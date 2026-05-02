package util

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPagination_Apply(t *testing.T) {
	tests := []struct {
		name     string
		p        Pagination
		length   int
		expected [2]int
	}{
		{"empty", Pagination{}, 100, [2]int{0, 100}},
		{"offset", Pagination{Offset: 10}, 100, [2]int{10, 100}},
		{"limit", Pagination{Limit: 20}, 100, [2]int{0, 20}},
		{"offset_limit", Pagination{Offset: 10, Limit: 20}, 100, [2]int{10, 30}},
		{"out_of_bounds_offset", Pagination{Offset: 200}, 100, [2]int{100, 100}},
		{"negative_offset", Pagination{Offset: -10}, 100, [2]int{0, 100}},
		{"extreme_limit", Pagination{Limit: 2000}, 100, [2]int{0, 100}},
		{"zero_limit", Pagination{Limit: 0}, 100, [2]int{0, 100}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, e := tt.p.Apply(tt.length)
			if s != tt.expected[0] || e != tt.expected[1] {
				t.Errorf("Apply(%d) = (%d, %d), expected (%d, %d)", tt.length, s, e, tt.expected[0], tt.expected[1])
			}
		})
	}
}

func TestSetupStandardLogging_EnvVars(t *testing.T) {
	// 1. Test different log levels
	levels := []string{"DEBUG", "INFO", "WARN", "ERROR", "CRITICAL"}
	for _, level := range levels {
		os.Setenv("ORCHESTRATOR_LOG_LEVEL", level)
		cleanup := SetupStandardLogging("test_level_"+level, nil)
		cleanup()
	}
	os.Unsetenv("ORCHESTRATOR_LOG_LEVEL")

	// 2. Test text format
	os.Setenv("ORCHESTRATOR_LOG_FORMAT", "text")
	cleanup := SetupStandardLogging("test_format_text", nil)
	cleanup()
	os.Unsetenv("ORCHESTRATOR_LOG_FORMAT")

	// 3. Test MCP_LOG_FILE
	logFile := filepath.Join(t.TempDir(), "mcp_global.log")
	os.Setenv("MCP_LOG_FILE", logFile)
	cleanup2 := SetupStandardLogging("test_global_file", nil)
	slog.Info("test global log")
	time.Sleep(50 * time.Millisecond)
	cleanup2()
	os.Unsetenv("MCP_LOG_FILE")

	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("expected global log file to exist")
	}
}

func TestAsyncWriter_Timeout(t *testing.T) {
	var buf bytes.Buffer
	// Small capacity, 0 timeout (or very small) to trigger drop
	aw := NewAsyncWriter(&buf, 1)
	aw.maxDuration = 1 * time.Microsecond

	// Fill channel
	aw.Write([]byte("msg1\n"))

	// This should timeout and drop
	aw.Write([]byte("msg2\n"))

	time.Sleep(50 * time.Millisecond)
	aw.Close()

	if aw.dropped == 0 {
		t.Error("expected dropped logs on timeout")
	}
}

func TestHardenedAddTool_WrapperLogic(t *testing.T) {
	os.Setenv("MCP_ORCHESTRATOR_OWNED", "true")
	defer os.Unsetenv("MCP_ORCHESTRATOR_OWNED")

	srv := mcp.NewServer(&mcp.Implementation{}, &mcp.ServerOptions{})
	sp := &MockSessionProvider{Srv: srv}
	tool := &mcp.Tool{
		Name: "test_tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"val": map[string]any{"type": "string"},
			},
		},
	}

	// 1. Test Heuristic Analysis (Success)
	h1 := func(ctx context.Context, req *mcp.CallToolRequest, in map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Operation successful"}},
		}, "output", nil
	}
	wrapper1 := HardenedAddTool(sp, tool, h1)
	res1, _, _ := wrapper1(context.Background(), &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}, map[string]any{})

	if len(res1.Content) != 2 {
		t.Errorf("expected 2 content items (result + signal), got %d", len(res1.Content))
	}
	sigText := res1.Content[1].(*mcp.TextContent).Text
	if !strings.Contains(sigText, `"success":true`) {
		t.Errorf("expected success signal, got %s", sigText)
	}

	// 2. Test Heuristic Analysis (Failure)
	h2 := func(ctx context.Context, req *mcp.CallToolRequest, in map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "no matches found"}},
		}, "output", nil
	}
	wrapper2 := HardenedAddTool(sp, tool, h2)
	res2, _, _ := wrapper2(context.Background(), &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}, map[string]any{})

	sigText2 := res2.Content[1].(*mcp.TextContent).Text
	if !strings.Contains(sigText2, `"success":false`) {
		t.Errorf("expected failure signal, got %s", sigText2)
	}

	// 3. Test Artifact Path Fast-Path
	tmpDir := t.TempDir()
	artPath := filepath.Join(tmpDir, "sub", "output.json")
	h3 := func(ctx context.Context, req *mcp.CallToolRequest, in map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "original"}}}, map[string]any{"key": "value"}, nil
	}
	wrapper3 := HardenedAddTool(sp, tool, h3)
	res3, out3, _ := wrapper3(context.Background(), &mcp.CallToolRequest{}, map[string]any{"artifact_path": artPath})

	if !strings.Contains(res3.Content[0].(*mcp.TextContent).Text, "Artifact written natively") {
		t.Errorf("expected artifact written message, got %v", res3.Content[0])
	}
	if out3 != nil {
		t.Errorf("expected output to be zeroed out, got %v", out3)
	}

	data, _ := os.ReadFile(artPath)
	if !strings.Contains(string(data), `"key": "value"`) {
		t.Errorf("expected file content to contain output, got %s", string(data))
	}

	// 4. Test Panic Recovery
	h4 := func(ctx context.Context, req *mcp.CallToolRequest, in map[string]any) (*mcp.CallToolResult, any, error) {
		panic("test panic")
	}
	wrapper4 := HardenedAddTool(sp, tool, h4)
	res4, _, err4 := wrapper4(context.Background(), &mcp.CallToolRequest{}, map[string]any{})
	if err4 != nil {
		t.Errorf("expected nil error on panic recovery, got %v", err4)
	}
	if !res4.IsError || !strings.Contains(res4.Content[0].(*mcp.TextContent).Text, "Panic recovered") {
		t.Errorf("expected error result on panic, got %v", res4)
	}
}

func TestHardenedAddTool_AutoHFSC(t *testing.T) {
	os.Setenv("MCP_ORCHESTRATOR_OWNED", "true")
	defer os.Unsetenv("MCP_ORCHESTRATOR_OWNED")

	srv := mcp.NewServer(&mcp.Implementation{}, &mcp.ServerOptions{})
	sp := &MockSessionProvider{Srv: srv}
	tool := &mcp.Tool{Name: "large_tool"}

	// Create a payload > 2MB
	largeData := make([]byte, 2100000)
	for i := range largeData {
		largeData[i] = 'a'
	}
	largeOut := string(largeData)

	h := func(ctx context.Context, req *mcp.CallToolRequest, in map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, largeOut, nil
	}

	wrapper := HardenedAddTool(sp, tool, h)
	// This will trigger auto-hfsc. Since sp.Session() is nil, StreamHeavyPayload will fallback to standalone mode
	// because it's orchestrated but session is nil.
	// Wait, hfsc.StreamHeavyPayload:
	// if isOrchestrated && session != nil { ... } else { ... return standalone ... }
	res, out, err := wrapper(context.Background(), &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "large_tool"}}, map[string]any{})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if out != nil {
		// In auto-hfsc branch, it returns *new(Out) which is nil for any
		// Wait, return hfscRes, *new(Out), nil
	}
	if res == nil || len(res.Content) == 0 {
		t.Errorf("expected hfsc result")
	}
}
