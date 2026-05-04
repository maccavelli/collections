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

// UniversalBaseInput defines the standard parameters required for all utility tool calls,
// ensuring telemetry correlation via SessionID without disrupting tool-specific parameters.
type UniversalBaseInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"Optional tracking ID"`
}

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

// HardenedAddTool registers an MCP tool with the server while automatically applying a recovery middleware.
// It uses generics to match the official SDK's AddTool signature while providing a panic-safe execution environment.
func HardenedAddTool[In any, Out any](
	s *mcp.Server,
	tool *mcp.Tool,
	handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error),
) {
	mcp.AddTool(s, tool, InternalWrapHandler(tool, handler))
}

// InternalWrapHandler wraps a generic ToolHandler in recovery + audit middleware to prevent panics
// from crashing the sub-server and to emit structured audit logs for every tool invocation.
func InternalWrapHandler[In any, Out any](
	tool *mcp.Tool,
	handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error),
) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (res *mcp.CallToolResult, output Out, err error) {
		start := time.Now()

		toolName := tool.Name
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

		res, output, err = handler(ctx, req, input)
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

			sigBytes, _ := json.Marshal(map[string]any{"__orchestrator_signal": signal})
			res.Content = append(res.Content, &mcp.TextContent{
				Text: string(sigBytes),
			})
		}
		// ---------------------------------------------------
		return res, output, err
	}
}
