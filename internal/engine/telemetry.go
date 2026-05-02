package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// CSSATelemetry defines the strict data transfer object for recall cross-server bounds
type CSSATelemetry struct {
	ReportFragment string `json:"report_fragment,omitempty"`
	TraceData      any    `json:"trace_data,omitempty"`
}

// PublishSessionToRecall pushes the current session state to recall for cross-server consumption.
// Submits analytical trace data enabling W3C telemetry processing globally.
// The session_id is a raw nonce; recall's save_sessions handler constructs the
// full compound key: {server_id}:session:{project_id}:{outcome}:{session_id}.
func (e *Engine) PublishSessionToRecall(ctx context.Context, sessionID, projectID, outcome, model, traceContext, reportFragment string, data any) {
	e.mu.RLock()
	client := e.ExternalClient
	e.mu.RUnlock()

	if client == nil || !client.RecallEnabled() {
		slog.Debug("PublishSessionToRecall skipped: recall not available")
		return
	}

	telemetryDto := CSSATelemetry{
		ReportFragment: reportFragment,
		TraceData:      data,
	}

	payload, err := json.Marshal(telemetryDto)
	if err != nil {
		slog.Warn("PublishSessionToRecall marshal error", "project", projectID, "err", err)
		return
	}

	// Pass raw nonce as session_id — recall's save_sessions builds the compound key.
	nonce := sessionID
	if nonce == "" {
		nonce = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	args := map[string]any{
		"server_id":     "brainstorm",
		"project_id":    projectID,
		"outcome":       outcome,
		"session_id":    nonce,
		"model":         model,
		"token_spend":   0, // Hook point for future LLM engine extension
		"trace_context": traceContext,
		"state_data":    string(payload),
	}

	result := client.CallDatabaseTool(ctx, "save_sessions", args)
	if result != "" {
		slog.Info("PublishSessionToRecall success", "project", projectID, "outcome", outcome, "nonce", nonce, "size", len(payload))
	}
}

// LoadCrossSessionFromRecall queries recall for historic session data published by a peer server (e.g., go-refactor).
// It retrieves the entire trace dataset for the project, enabling 1:N analytical pattern discovery.
func (e *Engine) LoadCrossSessionFromRecall(ctx context.Context, peerServer, projectID string) string {
	e.mu.RLock()
	client := e.ExternalClient
	e.mu.RUnlock()

	if client == nil || !client.RecallEnabled() {
		slog.Debug("LoadCrossSessionFromRecall skipped: recall not available")
		return ""
	}

	result := client.CallDatabaseTool(ctx, "list", map[string]any{"namespace": "sessions",
		"server_id":  peerServer,
		"project_id": projectID,
	})
	if result != "" && result != "No records found matching the specified parameters." && !strings.Contains(result, "Error") {
		slog.Info("LoadCrossSessionFromRecall hit", "peer", peerServer, "project", projectID, "size", len(result))
		return result
	}
	return ""
}
