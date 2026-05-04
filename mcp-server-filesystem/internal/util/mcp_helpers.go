package util

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"os"
	"strings"
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

// HardenedAddTool registers an MCP tool with the server while automatically applying a recovery middleware.
// It uses generics to match the official SDK's AddTool signature while providing a panic-safe execution environment.
func HardenedAddTool[In any, Out any](
	s *mcp.Server,
	tool *mcp.Tool,
	handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error),
) {
	mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, input In) (res *mcp.CallToolResult, output Out, err error) {
		defer func() {
			if r := recover(); r != nil {
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
		}()
		res, output, err = handler(ctx, req, input)

		// ---------------- TELEMETRY INJECTION ----------------
		if os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true" && res != nil {
			success := true
			confidence := 1.0

			// Basic generic heuristic analysis:
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
				IntentContext: req.Params.Name,
				Category:      tool.Name,
			}

			sigBytes, _ := json.Marshal(map[string]any{"__orchestrator_signal": signal})
			res.Content = append(res.Content, &mcp.TextContent{
				Text: string(sigBytes),
			})
		}
		// ---------------------------------------------------
		return res, output, err
	})
}
