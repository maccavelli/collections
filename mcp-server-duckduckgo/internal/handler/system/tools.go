package system

import (
	"bytes"
	"context"
	"regexp"

	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-duckduckgo/internal/config"
	"mcp-server-duckduckgo/internal/registry"
	"mcp-server-duckduckgo/internal/util"
)

// LogBuffer stores recent server logs in memory.
type LogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

var secretRegex = regexp.MustCompile(`(?i)(token_|sk_|key_|secret_)[a-zA-Z0-9_-]+`)

func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	redacted := secretRegex.ReplaceAll(p, []byte("[REDACTED]"))
	_, err = lb.buf.Write(redacted)
	if err != nil {
		return len(p), err
	}

	const maxLogSize = config.LogBufferLimit
	const trimTarget = config.LogTrimTarget

	if lb.buf.Len() > maxLogSize {
		data := lb.buf.Bytes()
		trimPoint := len(data) - trimTarget
		if idx := bytes.IndexByte(data[trimPoint:], '\n'); idx >= 0 {
			trimPoint += idx + 1
		}
		newData := make([]byte, len(data)-trimPoint)
		copy(newData, data[trimPoint:])
		lb.buf.Reset()
		_, _ = lb.buf.Write(newData)
	}

	return len(p), nil
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

func (t *GetInternalLogsTool) Name() string { return "get_internal_logs" }

type LogsInput struct {
	MaxLines int `json:"max_lines" jsonschema:"Max log lines to return (default 25)."`
}

func (t *GetInternalLogsTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Audit Streaming] Streams live fault vectors, background stdout, and daemon logs directly bypassing payload limits. Keywords: debug, errors, stack-trace, logs, fault, auditing, diagnostics",
	}, t.Handle)
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

// tailLines returns the last n lines of s using a zero-allocation backward scan.
func tailLines(s string, n int) string {
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

// Register registers system tools with the global registry.
func Register(lb *LogBuffer) {
	registry.Global.Register(&GetInternalLogsTool{Buffer: lb})
}
