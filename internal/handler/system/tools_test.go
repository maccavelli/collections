package system

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLogBuffer(t *testing.T) {
	lb := &LogBuffer{}
	
	// Test basic write
	content := "line 1\nline 2\n"
	n, err := lb.Write([]byte(content))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(content) {
		t.Errorf("expected %d bytes, got %d", len(content), n)
	}
	if lb.String() != content {
		t.Errorf("expected %q, got %q", content, lb.String())
	}
}

func TestGetInternalLogsTool(t *testing.T) {
	lb := &LogBuffer{}
	lb.Write([]byte("line 1\nline 2\nline 3\nline 4\nline 5\n"))
	
	tool := &GetInternalLogsTool{Buffer: lb}
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		if tool.Name() != "get_internal_logs" {
			t.Errorf("expected name get_internal_logs, got %s", tool.Name())
		}
	})

	t.Run("Handle default", func(t *testing.T) {
		req := &mcp.CallToolRequest{}
		resp, _, err := tool.Handle(ctx, req, LogsInput{})
		if err != nil {
			t.Fatalf("Handle failed: %v", err)
		}
		if resp.IsError {
			t.Error("expected success")
		}
		
		text := ""
		if len(resp.Content) > 0 {
			if tc, ok := resp.Content[0].(*mcp.TextContent); ok {
				text = tc.Text
			}
		}
		
		if !strings.Contains(text, "line 1") || !strings.Contains(text, "line 5") {
			t.Errorf("unexpected log output: %s", text)
		}
	})

	t.Run("Handle max_lines", func(t *testing.T) {
		req := &mcp.CallToolRequest{}
		input := LogsInput{MaxLines: 2}
		resp, _, err := tool.Handle(ctx, req, input)
		if err != nil {
			t.Fatalf("Handle failed: %v", err)
		}
		
		text := ""
		if len(resp.Content) > 0 {
			if tc, ok := resp.Content[0].(*mcp.TextContent); ok {
				text = tc.Text
			}
		}

		lines := strings.Split(strings.TrimSpace(text), "\n")
		if len(lines) != 2 {
			t.Errorf("expected 2 lines, got %d: %q", len(lines), text)
		}
		if lines[0] != "line 4" || lines[1] != "line 5" {
			t.Errorf("unexpected lines: %v", lines)
		}
	})
}

func TestTailLines(t *testing.T) {
	input := "1\n2\n3\n4\n"
	
	tests := []struct {
		n        int
		expected string
	}{
		{1, "4"},
		{2, "3\n4"},
		{5, "1\n2\n3\n4"},
		{0, ""},
	}

	for _, tc := range tests {
		got := tailLines(input, tc.n)
		if got != tc.expected {
			t.Errorf("tailLines(%q, %d) = %q; expected %q", input, tc.n, got, tc.expected)
		}
	}
}
