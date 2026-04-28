package system

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-server-sequential-thinking/internal/config"
	"mcp-server-sequential-thinking/internal/registry"
	"mcp-server-sequential-thinking/internal/util"
)

// LogBuffer stores recent server logs in memory.
type LogBuffer struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

var secretRegex = regexp.MustCompile(`(?i)(token_|sk_|key_|secret_)[a-zA-Z0-9_-]+`)

// Write appends a chunk of logs to the buffer with an upper limit safety.
func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	redacted := secretRegex.ReplaceAll(p, []byte("[REDACTED]"))
	n, err = lb.buf.Write(redacted)
	if err != nil {
		return n, err
	}

	if lb.buf.Len() > config.LogBufferLimit {
		data := lb.buf.Bytes()
		trimPoint := len(data) - config.LogTrimTarget
		if idx := bytes.IndexByte(data[trimPoint:], '\n'); idx >= 0 {
			trimPoint += idx + 1
		}
		remaining := data[trimPoint:]
		lb.buf.Reset()
		if _, err := lb.buf.Write(remaining); err != nil {
			return 0, fmt.Errorf("trim buffer: %w", err)
		}
	}

	return n, nil
}

// String returns the contiguous chunk of safely buffered logs.
func (lb *LogBuffer) String() string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.buf.String()
}

// GetInternalLogsTool handles log retrieval.
type GetInternalLogsTool struct {
	Buffer *LogBuffer
}

// Name returns the MCP tool identifier for reading logs.
func (t *GetInternalLogsTool) Name() string {
	return "get_internal_logs"
}

// Register exposes the internal logs tool to the MCP server.
func (t *GetInternalLogsTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Audit Streaming] Streams live fault vectors, background stdout, and daemon logs directly for system diagnostics. Keywords: debug, errors, stack-trace, logs, fault, auditing, diagnostics",
	}, t.Handle)
}

// LogsInput defines the configuration structure for the GetInternalLogsTool.
type LogsInput struct {
	MaxLines int `json:"max_lines,omitzero" jsonschema:"Max log lines to return (default 25)."`
}

// Handle retrieves logs from the central buffer up to a specified line limit.
func (t *GetInternalLogsTool) Handle(_ context.Context, req *mcp.CallToolRequest, input LogsInput) (*mcp.CallToolResult, any, error) {
	maxLines := config.DefaultLogLines
	if input.MaxLines > 0 {
		maxLines = input.MaxLines
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: tailLines(t.Buffer.String(), maxLines)}},
	}, nil, nil
}

// tailLines returns the last n lines of s using a zero-allocation backward scan.
func tailLines(s string, n int) string {
	// Strip trailing newline to match previous behavior
	if len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	if len(s) == 0 {
		return ""
	}
	count := 0
	i := len(s)
	for i > 0 {
		i--
		if s[i] == '\n' {
			count++
			if count == n {
				return s[i+1:]
			}
		}
	}
	return s
}

// Register adds the system tools to the registry.
func Register(buffer *LogBuffer) {
	registry.Global.Register(&GetInternalLogsTool{Buffer: buffer})
}
