// Package decision provides functionality for the decision subsystem.
package decision

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
)

// CaptureDecisionTool handles decision ADR generation.
type CaptureDecisionTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

// Name performs the Name operation.
func (t *CaptureDecisionTool) Name() string {
	return "capture_decision_logic"
}

// Register performs the Register operation.
func (t *CaptureDecisionTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: SYNTHESIZER] ADR GENERATOR: Creates RFC-style Architecture Decision Records documenting the chosen approach, rejected alternatives, and operational consequences.  [PIPELINE CONSTRAINT: Do not invoke autonomously. This endpoint may only be queried as a strict coordinate sequence provided explicitly by magictools:compose_pipeline.]",
	}, t.Handle)
}

// DecisionInput defines the DecisionInput structure.
type DecisionInput struct {
	models.UniversalPipelineInput
}

// Handle performs the Handle operation.
func (t *CaptureDecisionTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DecisionInput) (*mcp.CallToolResult, any, error) {
	var standards string
	session, sessionErr := t.Manager.LoadSession(ctx)
	if sessionErr == nil && session != nil {
		if stds, ok := session.Metadata["standards"].(string); ok {
			standards = stds
		}
	}

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	// Load historical ADR context from recall.
	if recallAvailable && sessionErr == nil && session != nil && session.ProjectRoot != "" {
		if adrHistory := t.Engine.LoadCrossSessionFromRecall(ctx, "brainstorm", session.ProjectRoot); adrHistory != "" {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["historical_adrs"] = adrHistory
		}
	}

	// TraceMap enrichment for code-grounded ADR generation.
	if recallAvailable && sessionErr == nil && session != nil && session.ProjectRoot != "" {
		if tm, tmErr := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "go-refactor", session.ProjectRoot); tmErr == nil && tm != nil {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["go_refactor_trace"] = tm
		}
	}

	alternatives := ""
	if alt, ok := input.Flags["alternatives"].(string); ok {
		alternatives = alt
	}

	adr, err := t.Engine.CaptureDecisionLogic(ctx, input.Context, alternatives, standards)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	if recallAvailable && sessionErr == nil && session != nil && session.Metadata != nil {
		sessionKey := session.ProjectRoot
		if sessionKey == "" {
			sessionKey = "default"
		}
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, sessionKey, "completed", "native", "decision_capture", "", session.Metadata)
	}

	var returnData any = adr
	var returnSummary string = adr.Summary

	if input.SessionID != "" && recallAvailable {
		saveErr := t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, adr)
		if saveErr == nil {
			returnSummary += fmt.Sprintf("\n[CSSA STATUS]: Complete structural data saved successfully to recall session '%s'", input.SessionID)
			returnData = struct {
				Summary string `json:"summary"`
				Data    any    `json:"data,omitempty"`
			}{
				Summary: returnSummary,
				Data:    nil,
			}
		} else {
			returnSummary += "\n[CSSA STATUS]: Could not save to recall. Falling back to standard JSON-RPC."
			adr.Summary = returnSummary
			returnData = adr
		}
	}

	return &mcp.CallToolResult{}, returnData, nil
}

// Register adds the decision tools to the registry.
func Register(mgr *state.Manager, eng *engine.Engine) {
	// Purged to SynthesisGeneratorTool macro.
}
