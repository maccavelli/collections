// Package util provides functionality for the util subsystem.
package util

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/invopop/jsonschema"
	"mcp-server-brainstorm/internal/hfsc"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var globalToolMu sync.Mutex

// NopReadCloser defines the NopReadCloser structure.
type NopReadCloser struct{ io.Reader }

// Close performs the Close operation.
func (n NopReadCloser) Close() error { return nil }

// NopWriteCloser defines the NopWriteCloser structure.
type NopWriteCloser struct{ io.Writer }

// Close performs the Close operation.
func (n NopWriteCloser) Close() error { return nil }

// Pagination provides standard bounding for standalone tool execution constraints.
type Pagination struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"Optional cumulative session correlation ID. Pass this to accumulate extreme structural analysis payloads securely in the background."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum items to return. Defaults to 500 if unassigned."`
	Offset    int    `json:"offset,omitempty" jsonschema:"Pagination offset slice start."`
}

// Apply scales the length input strictly within the bounded limits defensively.
func (p *Pagination) Apply(length int) (start, end int) {
	limit := p.Limit
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	start = min(max(p.Offset, 0), length)
	end = min(start+limit, length)
	return start, end
}

// SessionProvider defines the SessionProvider structure.
type SessionProvider interface {
	MCPServer() *mcp.Server
	Session() *mcp.ServerSession
}

// MockSessionProvider defines the MockSessionProvider structure.
type MockSessionProvider struct{ Srv *mcp.Server }

// MCPServer performs the MCPServer operation.
func (m *MockSessionProvider) MCPServer() *mcp.Server { return m.Srv }

// Session performs the Session operation.
func (m *MockSessionProvider) Session() *mcp.ServerSession { return nil }

// HardenedAddTool registers an MCP tool with the server while automatically applying a recovery middleware.
// It uses generics to match the official SDK's AddTool signature while providing a panic-safe execution environment.
func HardenedAddTool[In any, Out any](
	sp SessionProvider,
	tool *mcp.Tool,
	handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error),
) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	// Dynamically derive schema if omitted using reflection on the generic input type 'In'
	if tool.InputSchema == nil {
		r := new(jsonschema.Reflector)
		r.ExpandedStruct = true
		sch := r.Reflect(new(In))
		b, _ := json.Marshal(sch)
		var schemaMap map[string]any
		json.Unmarshal(b, &schemaMap)
		tool.InputSchema = schemaMap
	}

	s := sp.MCPServer()
	wrapper := func(ctx context.Context, req *mcp.CallToolRequest, input In) (res *mcp.CallToolResult, output Out, err error) {
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

		globalToolMu.Lock()
		defer globalToolMu.Unlock()

		res, output, err = handler(ctx, req, input)

		// ---------------- TELEMETRY INJECTION ----------------
		isOrchestrated := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"

		// Option A: Universal ArtifactPath Native Fast-Path Routing
		if isOrchestrated {
			var artifactPath string
			if inStr, mErr := json.Marshal(input); mErr == nil {
				var inMap map[string]any
				if json.Unmarshal(inStr, &inMap) == nil {
					if p, ok := inMap["artifact_path"].(string); ok && strings.TrimSpace(p) != "" {
						artifactPath = strings.TrimSpace(p)
					}
				}
			}

			if artifactPath != "" {
				if structBytes, mErr := json.MarshalIndent(output, "", "  "); mErr == nil && len(structBytes) > 2 {
					_ = os.MkdirAll(filepath.Dir(artifactPath), 0o755)
					if wErr := os.WriteFile(artifactPath, structBytes, 0o644); wErr == nil {
						slog.Info("artifact fast-path os route successful", "path", artifactPath, "size", len(structBytes))
						if res == nil {
							res = &mcp.CallToolResult{}
						}
						res.Content = []mcp.Content{
							&mcp.TextContent{Text: fmt.Sprintf("Artifact written natively to: %s", artifactPath)},
						}
						return res, *new(Out), err // Zero-out standard output
					}
				}
			}
		}

		// Option C: Implicit Sub-Server Auto-HFSC
		if isOrchestrated {
			if structBytes, marshalErr := json.Marshal(output); marshalErr == nil && len(structBytes) > 2000000 {
				var logSess hfsc.LogSession
				if sess := sp.Session(); sess != nil {
					logSess = sess
				}
				buffer := bytes.NewReader(structBytes)
				hfscRes, hfscErr := hfsc.StreamHeavyPayload(ctx, logSess, "extreme_auto_intercept.json", req.Params.Name, "auto", buffer)
				if hfscErr == nil {
					slog.Info("auto-hfsc: natively intercepted extreme payload", "size", len(structBytes))
					return hfscRes, *new(Out), nil // return zero value for Output so SDK serialization is bypassed
				}
			}
		}

		if isOrchestrated && res != nil {
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
		// 🛡️ Pure JSON Mandate: If structured content is generated (non-nil output),
		// initialize Content as an empty slice to satisfy SDK's non-nil check and
		// prevent hybrid TextContent generation in the backplane. This only applies
		// if the orchestrator is natively handling StructuredContent proxying.
		if isOrchestrated {
			if err == nil && res != nil && res.Content == nil {
				res.Content = []mcp.Content{}
			}
		}

		return res, output, err
	}
	mcp.AddTool(s, tool, wrapper)
	return wrapper
}
