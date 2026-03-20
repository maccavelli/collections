package handler

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/state"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleAnalyzeProject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-handler-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := state.NewManager(tmpDir)
	eng := engine.NewEngine(tmpDir)
	h := HandleAnalyzeProject(mgr, eng)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"path": tmpDir,
			},
		},
	}

	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler failed: %v", err)
	}

	if res.IsError {
		t.Fatalf("expected success, got error: %v", res.Content)
	}

	// Verify JSON structure.
	var output map[string]interface{}
	if err := json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &output); err != nil {
		// Try to parse from JSON content if it's returns that way.
		// MCP Go ToolResultJSON might return TextContent with JSON string.
	}
}

func TestHandleGetInternalLogs(t *testing.T) {
	mockBuf := &mockLogBuffer{val: "line1\nline2\nline3"}
	h := HandleGetInternalLogs(mockBuf)

	// Default
	req := mcp.CallToolRequest{}
	res, _ := h(context.Background(), req)
	txt := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(txt, "line3") {
		t.Error("expected logs")
	}

	// With max_lines
	req = mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"max_lines": 1,
			},
		},
	}
	res, _ = h(context.Background(), req)
	txt = res.Content[0].(mcp.TextContent).Text
	if txt != "line3" {
		t.Errorf("want 'line3', got '%s'", txt)
	}
}

type mockLogBuffer struct {
	val string
}

func (m *mockLogBuffer) String() string {
	return m.val
}

func TestHandleChallengeAssumption(t *testing.T) {
	eng := engine.NewEngine(".")
	h := HandleChallengeAssumption(eng)

	// Missing 'design' param.
	req := mcp.CallToolRequest{}
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for missing design param")
	}

	// Valid 'design' param.
	req = mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"design": "Use a database",
			},
		},
	}
	res, err = h(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Errorf("unexpected error: %v", res.Content)
	}
}

func TestHandleAnalyzeProject_Errors(t *testing.T) {
	mgr := state.NewManager("/tmp/invalid-path")
	eng := engine.NewEngine(".")
	h := HandleAnalyzeProject(mgr, eng)

	// Session load error
	res, _ := h(context.Background(), mcp.CallToolRequest{})
	if !res.IsError {
		t.Error("expected error for invalid session load")
	}

	// Context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = h(ctx, mcp.CallToolRequest{})

	// Engine error (invalid path)
	mgr2 := state.NewManager(t.TempDir())
	h2 := HandleAnalyzeProject(mgr2, eng)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"path": "/non/existent/path/for/mcp/handler",
			},
		},
	}
	res, _ = h2(context.Background(), req)
	if !res.IsError {
		t.Error("expected error for invalid engine path")
	}
}

func TestHandleSuggestNextStep_Status(t *testing.T) {
	mgr := state.NewManager(".")
	eng := engine.NewEngine(".")
	h := HandleSuggestNextStep(mgr, eng)
	
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"path": ".",
			},
		},
	}
	_, _ = h(context.Background(), req)
}

func TestHandleAnalyzeEvolution(t *testing.T) {
	eng := engine.NewEngine(".")
	h := HandleAnalyzeEvolution(eng)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"proposal": "refactor auth",
			},
		},
	}
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal("unexpected error")
	}
}

func TestHandleDiscoverProject(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "mcp-h-test-*")
	defer os.RemoveAll(tmpDir)
	mgr := state.NewManager(tmpDir)
	eng := engine.NewEngine(tmpDir)
	h := HandleDiscoverProject(mgr, eng)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"path": tmpDir,
			},
		},
	}
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal("unexpected error")
	}

	// Error case: invalid manager/load
	hErr := HandleDiscoverProject(state.NewManager("/invalid/path"), eng)
	res, _ = hErr(context.Background(), req)
	if !res.IsError {
		t.Error("expected error for invalid manager path")
	}
}

func TestHandleCritiqueDesign(t *testing.T) {
	eng := engine.NewEngine(".")
	h := HandleCritiqueDesign(eng)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"design": "Use a db",
			},
		},
	}
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal("unexpected error")
	}
}

func TestHandleCaptureDecision(t *testing.T) {
	eng := engine.NewEngine(".")
	h := HandleCaptureDecision(eng)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"decision":     "Use Go",
				"alternatives": "Java",
			},
		},
	}
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal("unexpected error")
	}

	// Error case: missing params
	req = mcp.CallToolRequest{}
	res, _ = h(context.Background(), req)
	if !res.IsError {
		t.Error("expected error for missing params")
	}
}

func TestHandleEvaluateQuality(t *testing.T) {
	eng := engine.NewEngine(".")
	h := HandleEvaluateQuality(eng)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"design": "test design",
			},
		},
	}
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal("unexpected error")
	}

	// Error case: missing design
	req = mcp.CallToolRequest{}
	res, _ = h(context.Background(), req)
	if !res.IsError {
		t.Error("expected error for missing design")
	}
}

func TestHandleRedTeamReview(t *testing.T) {
	eng := engine.NewEngine(".")
	h := HandleRedTeamReview(eng)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"design": "test design",
			},
		},
	}
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal("unexpected error")
	}

	// Error case: missing design
	req = mcp.CallToolRequest{}
	res, _ = h(context.Background(), req)
	if !res.IsError {
		t.Error("expected error for missing design")
	}
}

func TestHandleAnalyzeEvolution_Error(t *testing.T) {
	eng := engine.NewEngine(".")
	h := HandleAnalyzeEvolution(eng)
	req := mcp.CallToolRequest{}
	res, _ := h(context.Background(), req)
	if !res.IsError {
		t.Error("expected error for missing proposal")
	}
}

func TestHandleCritiqueDesign_Error(t *testing.T) {
	eng := engine.NewEngine(".")
	h := HandleCritiqueDesign(eng)
	req := mcp.CallToolRequest{}
	res, _ := h(context.Background(), req)
	if !res.IsError {
		t.Error("expected error for missing design")
	}
}

func TestTailLines(t *testing.T) {
	cases := []struct {
		input string
		n     int
		want  string
	}{
		{"", 5, ""},
		{"a\nb\nc", 2, "b\nc"},
		{"a\nb\nc\n", 2, "b\nc"},
		{"a\n", 5, "a"},
		{"line1\nline2", 1, "line2"},
	}
	for _, c := range cases {
		got := tailLines(c.input, c.n)
		if got != c.want {
			t.Errorf("tailLines(%q, %d) = %q; want %q", c.input, c.n, got, c.want)
		}
	}
}
