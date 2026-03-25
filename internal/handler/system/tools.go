package system

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-brainstorm/internal/config"
	"mcp-server-brainstorm/internal/registry"
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

func (t *GetInternalLogsTool) Metadata() mcp.Tool {
	return mcp.NewTool("get_internal_logs",
		mcp.WithDescription("Provides access to the server's internal diagnostic stream and audit trail, including detailed engine logs and tool execution metadata. This is vital for troubleshooting unexpected tool behavior or auditing the decision-making steps taken by the AI. Use this for debugging and verifying internal server state."),
		mcp.WithNumber("max_lines", mcp.Description("Max log lines to return (default 25).")),
	)
}

func (t *GetInternalLogsTool) Handle(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	maxLines := config.DefaultLogLines
	if v := req.GetInt("max_lines", 0); v > 0 {
		maxLines = v
	}
	return mcp.NewToolResultText(
		tailLines(t.Buffer.String(), maxLines),
	), nil
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
