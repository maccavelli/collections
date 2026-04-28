package util

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type NopReadCloser struct{ io.Reader }

func (n NopReadCloser) Close() error { return nil }

type NopWriteCloser struct{ io.Writer }

func (n NopWriteCloser) Close() error { return nil }

// contextKey is an unexported type for context keys in this package.
type contextKey string

const clientContextKey contextKey = "mcp_client"

// WithClient returns a copy of ctx with the MCP client identity attached.
func WithClient(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, clientContextKey, name)
}

// ClientFromContext extracts the MCP client identity from the context.
// Returns "stdio" if no client identity was set (i.e. the request came via the stdio backplane).
func ClientFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(clientContextKey).(string); ok && v != "" {
		return v
	}
	return "stdio"
}

// SafeToolHandler wraps a ToolHandler in recovery + audit middleware to prevent panics
// from crashing the sub-server and to emit structured audit logs for every tool invocation.
func SafeToolHandler(handler func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (res *mcp.CallToolResult, err error) {
		start := time.Now()

		toolName := ""
		func() {
			defer func() { recover() }()
			if req != nil {
				toolName = req.Params.Name
			}
		}()

		client := ClientFromContext(ctx)

		defer func() {
			elapsed := time.Since(start)
			isErr := (err != nil) || (res != nil && res.IsError)

			if r := recover(); r != nil {
				isErr = true
				res = &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{
						&mcp.TextContent{
							Text: fmt.Sprintf("Internal Error: Panic recovered in handler: %v", r),
						},
					},
				}
				err = nil
			}

			slog.Info("audit",
				"client", client,
				"tool", toolName,
				"latency_ms", elapsed.Milliseconds(),
				"ok", !isErr,
			)
		}()

		res, err = handler(ctx, req)
		if err == nil && res != nil {
			// 🛡️ Pure JSON Mandate: If structured content is provided, initialize Content
			// as an empty slice to satisfy SDK checks and prevent hybrid text generation.
			if res.StructuredContent != nil && res.Content == nil {
				res.Content = []mcp.Content{}
			}
		}

		// ---------------- TELEMETRY INJECTION ----------------
		if os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true" && res != nil {
			success := true
			confidence := 1.0

			for _, content := range res.Content {
				if tc, ok := content.(*mcp.TextContent); ok {
					msg := strings.ToLower(tc.Text)
					if strings.Contains(msg, "no matches found") || strings.Contains(msg, "target not identified") || strings.Contains(msg, "could not find") {
						success = false
						confidence = 0.5
						break
					}
				}
			}

			signal := struct {
				Success       bool    `json:"success"`
				Confidence    float64 `json:"confidence"`
				IntentContext string  `json:"intent_context"`
				Category      string  `json:"category"`
			}{
				Success:       success,
				Confidence:    confidence,
				IntentContext: toolName,
				Category:      toolName, // Using extracted local toolName
			}

			sigBytes, _ := json.Marshal(map[string]interface{}{"__orchestrator_signal": signal})
			res.Content = append(res.Content, &mcp.TextContent{
				Text: string(sigBytes),
			})
		}
		// ---------------------------------------------------
		return res, err
	}
}
