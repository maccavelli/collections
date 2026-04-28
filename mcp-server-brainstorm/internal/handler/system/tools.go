package system

import (
	"bytes"
	"context"
	"fmt"
	"mcp-server-brainstorm/internal/util"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/config"
	"mcp-server-brainstorm/internal/registry"
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
	n = len(p)

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

func (t *GetInternalLogsTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: DIAGNOSTIC] SYSTEM LOG INSPECTOR: Provides access to system logs and bug debugging trails for troubleshooting and auditing AI decision-making steps. [PIPELINE CONSTRAINT: Do not invoke autonomously. Must map strictly to pipeline limits.] [Routing Tags: debug, logs, system-stdout, diagnostics]",
	}, t.Handle)
}

type LogsInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"CSSA backend storage pipeline correlation ID."`
	MaxLines  int    `json:"max_lines" jsonschema:"Max log lines to return (default 25)."`
}

func (t *GetInternalLogsTool) Handle(_ context.Context, _ *mcp.CallToolRequest, input LogsInput) (*mcp.CallToolResult, any, error) {
	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	if isOrchestrator {
		// Native anchoring for telemetry and trace logging if orchestrated natively.
		if input.SessionID != "" {
			_ = input.SessionID // standard CSSA mappings acknowledged safely
		}
	}

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
