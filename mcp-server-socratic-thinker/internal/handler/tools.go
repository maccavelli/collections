// Package handler provides functionality for the handler subsystem.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/go-playground/validator/v10"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-socratic-thinker/internal/socratic"
	"mcp-server-socratic-thinker/internal/telemetry"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// LogsRequest defines the LogsRequest structure.
type LogsRequest struct {
	MaxLines *int `json:"max_lines,omitempty"`
}

// Register explicitly maps the tools and resources to the MCP server.
func Register(mcpServer *mcp.Server, machine *socratic.Machine, ringBuffer *telemetry.RingBuffer) {
	// 1. Socratic Thinker Tool
	socraticTool := mcp.Tool{
		Name: "socratic_thinker",
		Description: `MANDATORY STATE MACHINE: Processes complex logic via a 4-stage sequential pipeline. You MUST use this tool; DO NOT simulate the logic in your own thought block.
1. INITIALIZE: Call the tool with stage="INITIALIZE" and provide the 'problem' string.
2. EXECUTE: The tool will return explicit instructions for the next stage.
3. RECURSE: You MUST pass your resulting analysis BACK INTO THIS TOOL one stage at a time, using the correct 'stage' and 'lemma' or 'aporia_synthesis' fields as instructed. Do not output your intermediate analysis to the user.
4. WAIT: Do not attempt to batch stages.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"stage": map[string]any{
					"type":        "string",
					"enum":        []string{"INITIALIZE", "THESIS", "ANTITHESIS_INITIAL", "THESIS_DEFENSE", "ANTITHESIS_EVALUATE", "CHAOS", "APORIA", "RESET"},
					"description": "The exact Socratic pipeline stage you are invoking.",
				},
				"problem": map[string]any{
					"type":        "string",
					"description": "The raw string problem. Only provide this during the INITIALIZE stage.",
				},
				"lemma": map[string]any{
					"type":        "string",
					"description": "Your isolated one-sentence summary for the active stage. Do not include your detailed 'Chain of Thought' here; perform all detailed reasoning natively in your markdown response before invoking this tool.",
				},
				"is_satisfied": map[string]any{
					"type":        "boolean",
					"description": "Used only in ANTITHESIS_EVALUATE to signal if the Thesis defense successfully resolved the tension. Acts as the Convergence Logic Gate.",
				},
				"aporia_synthesis": map[string]any{
					"type":        "string",
					"description": "Your final synthesized solution. Only provide this during the APORIA stage. Do not include your detailed 'Chain of Thought' here; perform all detailed reasoning natively in your markdown response before invoking this tool.",
				},
				"synthesis_critique": map[string]any{
					"type":        "string",
					"description": "Explicit self-correction and rigorous evaluation of the synthesis attempt. Only provide this during the APORIA stage.",
				},
				"paradox_detected": map[string]any{
					"type":        "boolean",
					"description": "Must be set to true if SynthesisCritique identifies a paradox, flaw, or assumption that breaks the synthesis. Only provide this during the APORIA stage.",
				},
				"resolution_strategy": map[string]any{
					"type":        "string",
					"description": "Mandatory step outlining how to fix the logic ONLY if paradox_detected is true.",
				},
			},
			"required": []string{"stage"},
		},
	}

	mcpServer.AddTool(&socraticTool, withRecovery(ringBuffer, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req socratic.Request
		if err := json.Unmarshal(request.Params.Arguments, &req); err != nil {
			return errorResult(fmt.Sprintf("JSON parse error: %v", err)), nil
		}
		if err := validate.Struct(req); err != nil {
			return errorResult(fmt.Sprintf("Validation error: %v", err)), nil
		}

		output, err := machine.Process(ctx, req)
		if err != nil && (err.Error() == "invalid stage" || err.Error() == "missing lemma" || err.Error() == "missing aporia_synthesis") {
			// Soft error from machine - return it as text so the agent sees the formatting hint
			return textResult(output), nil
		} else if err != nil {
			// e.g. context cancellation
			return errorResult(err.Error()), nil
		}

		return textResult(output), nil
	}))

	// 2. Internal Logs Tool
	logsTool := mcp.Tool{
		Name:        "get_internal_logs",
		Description: "Retrieves logs from the ring buffer to diagnose runtime issues.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"max_lines": map[string]any{
					"type":        "integer",
					"description": "Maximum lines of logs to return (default: 50).",
				},
			},
		},
	}

	mcpServer.AddTool(&logsTool, withRecovery(ringBuffer, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return textResult(ringBuffer.String()), nil
	}))

	// 3. Telemetry Resource (Hybrid Observability)
	res := mcp.Resource{
		Name:        "Socratic Logs",
		URI:         "socratic-thinker://logs",
		Description: "Server diagnostic telemetry logs",
		MIMEType:    "text/plain",
	}
	mcpServer.AddResource(&res, func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      request.Params.URI,
					MIMEType: "text/plain",
					Text:     ringBuffer.String(),
				},
			},
		}, nil
	})
}

func textResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
	}
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
	}
}

// withRecovery ensures that unhandled panics do not crash the server and corrupt stdio.
func withRecovery(ring *telemetry.RingBuffer, handler func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				trace := string(debug.Stack())
				slog.Error("Tool execution panic caught", "panic", r, "trace", trace)
				// The slog.Error will automatically get picked up by the ring buffer
				// since we hijack log.SetOutput in main, but we can also manually ensure it.
				if ring != nil {
					ring.AddLog("CRITICAL", fmt.Sprintf("Tool execution panic caught: %v\n%s", r, trace))
				}

				result = errorResult("Server encountered a fatal internal error: " + fmt.Sprintf("%v", r))
			}
		}()
		return handler(ctx, req)
	}
}
