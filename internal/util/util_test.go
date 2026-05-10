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

	// Write 60MB of data (above the 10MB truncation threshold)
	f, _ := os.Create(tmpFile)
	data := make([]byte, 60*1024*1024)
	for i := range data {
		data[i] = 'x'
	}
	// Insert newlines so truncation can snap to a boundary
	for i := 1024; i < len(data); i += 1024 {
		data[i] = '\n'
	}
	_, _ = f.Write(data)
	f.Close()

	hf := OpenHardenedLogFile(tmpFile)
	defer hf.Close()

	info, _ := os.Stat(tmpFile)
	// Graceful truncation retains ~5MB tail. Allow some tolerance for newline snapping.
	const truncateTarget = 5 * 1024 * 1024
	if info.Size() > int64(truncateTarget+1024) {
		t.Errorf("expected file to be truncated to ~%d bytes, got %d", truncateTarget, info.Size())
	}
	if info.Size() == 0 {
		t.Error("expected file to retain tail data, got 0 bytes")
	}
}

func TestHardenedAddTool_DeepClosure(t *testing.T) {
	tool := &mcp.Tool{Name: "test-tool"}

	// Test Success Path
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
	if len(res.Content) != 1 {
		t.Errorf("expected 1 content item, got %d", len(res.Content))
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
