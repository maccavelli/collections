package util

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAsyncWriter_Buffer(t *testing.T) {
	var buf bytes.Buffer
	aw := NewAsyncWriter(&buf, 10)

	input := []byte("hello world")
	_, err := aw.Write(input)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	err = aw.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), input) {
		t.Errorf("expected %s, got %s", input, buf.Bytes())
	}
}

func TestOpenHardenedLogFile_Cap(t *testing.T) {
	tmpFile := t.TempDir() + "/test2.log"

	f, _ := os.Create(tmpFile)
	_ = f.Truncate(60 * 1024 * 1024) // 60MB
	f.Close()

	hf := OpenHardenedLogFile(tmpFile)
	defer hf.Close()

	info, _ := os.Stat(tmpFile)
	if info.Size() != 0 {
		t.Errorf("expected file to be truncated to 0, got %d", info.Size())
	}
}

func TestHardenedAddTool_DeepClosure(t *testing.T) {
	t.Setenv("MCP_ORCHESTRATOR_OWNED", "true")

	tool := &mcp.Tool{Name: "test-tool"}

	// Test Success Path with Telemetry
	handler := func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, struct{}, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "all clear"}},
		}, struct{}{}, nil
	}

	wrapper := InternalWrapHandler(tool, handler)
	res, _, err := wrapper(context.Background(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: "test-tool"},
	}, struct{}{})

	if err != nil {
		t.Fatalf("wrapper failed: %v", err)
	}
	if len(res.Content) < 2 {
		t.Error("expected telemetry signal in content")
	}

	// Test "No matches found" heuristic
	negHandler := func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, struct{}, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "could not find any tool"}},
		}, struct{}{}, nil
	}
	negWrapper := InternalWrapHandler(tool, negHandler)
	resNeg, _, _ := negWrapper(context.Background(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: "test-tool"},
	}, struct{}{})

	sig := resNeg.Content[1].(*mcp.TextContent).Text
	if !strings.Contains(sig, `"success":false`) {
		t.Errorf("expected negative success signal, got: %s", sig)
	}

	// Test Panic Recovery
	panicHandler := func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, struct{}, error) {
		panic("recovered-panic")
	}
	panicWrapper := InternalWrapHandler(tool, panicHandler)
	resPanic, _, errPanic := panicWrapper(context.Background(), &mcp.CallToolRequest{}, struct{}{})
	if errPanic != nil {
		t.Errorf("expected error to be nil (recovered), got %v", errPanic)
	}
	if !resPanic.IsError || !strings.Contains(resPanic.Content[0].(*mcp.TextContent).Text, "Panic recovered") {
		t.Error("expected panic recovery message in result")
	}
}

func TestNopReadWriteCloser(t *testing.T) {
	var buf bytes.Buffer
	nrc := NopReadCloser{Reader: &buf}
	_ = nrc.Close()

	nwc := NopWriteCloser{Writer: &buf}
	_ = nwc.Close()
}
