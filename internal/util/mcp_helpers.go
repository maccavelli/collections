package util

import (
	"context"
	"fmt"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NopReadCloser defines the structural representation for the entity.
type NopReadCloser struct{ io.Reader }

func (n NopReadCloser) Close() error { return nil }

// NopWriteCloser defines the structural representation for the entity.
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
	mcp.AddTool(s, tool, InternalWrapHandler(tool, handler))
}

// InternalWrapHandler is exported for coverage testing of the closure logic.
func InternalWrapHandler[In any, Out any](
	tool *mcp.Tool,
	handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error),
) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (res *mcp.CallToolResult, output Out, err error) {
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

		if err == nil && res != nil && res.Content == nil {
			res.Content = []mcp.Content{}
		}
		return res, output, err
	}
}
