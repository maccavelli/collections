package system

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"mcp-server-go-refactor/internal/config"
	"mcp-server-go-refactor/internal/registry"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LogBuffer stores recent server logs in memory with
// ring-buffer semantics.
type LogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	n, err = lb.buf.Write(p)
	if err != nil {
		return n, err
	}

	if lb.buf.Len() > config.LogBufferLimit {
		data := lb.buf.Bytes()
		trimPoint := len(data) - config.LogTrimTarget
		if idx := bytes.IndexByte(data[trimPoint:], '\n'); idx >= 0 {
			trimPoint += idx + 1
		}
		newData := make([]byte, len(data)-trimPoint)
		copy(newData, data[trimPoint:])
		lb.buf.Reset()
		if _, err := lb.buf.Write(newData); err != nil {
			return 0, fmt.Errorf("trim buffer: %w", err)
		}
	}

	return n, nil
}

func (lb *LogBuffer) String() string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.buf.String()
}

// GetInternalLogsTool handles log retrieval.
type GetInternalLogsTool struct {
	Buffer *LogBuffer
}

func (t *GetInternalLogsTool) Name() string {
	return "get_internal_logs"
}

func (t *GetInternalLogsTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "SYSTEM MANDATE / AUDIT LOG: Provides a diagnostic window into the server's internal operations. Essential for troubleshooting multi-tool workflows and verifying server state. Call this if tool outputs are unexpected.",
	}, t.Handle)
}

type LogsInput struct {
	MaxLines int `json:"max_lines" jsonschema:"Max log lines to return (default 25)."`
}

func (t *GetInternalLogsTool) Handle(_ context.Context, req *mcp.CallToolRequest, input LogsInput) (*mcp.CallToolResult, any, error) {
	maxLines := config.DefaultLogLines
	if input.MaxLines > 0 {
		maxLines = input.MaxLines
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: tailLines(t.Buffer.String(), maxLines)}},
	}, nil, nil
}

// tailLines returns the last n lines of s.
func tailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// Register adds the system tools to the registry.
func Register(buffer *LogBuffer) {
	registry.Global.Register(&GetInternalLogsTool{Buffer: buffer})
}
