package security

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/registry"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
)

// ThreatModelerTool performs STRIDE-based threat modeling on system design.
type ThreatModelerTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

func (t *ThreatModelerTool) Name() string {
	return "threat_model_auditor"
}

func (t *ThreatModelerTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: THREAT] STRIDE THREAT MODELER: Performs automated STRIDE threat modeling on the system architecture (Spoofing, Tampering, etc.). [TRIGGERS: go-refactor:go_sql_injection_guard] [Routing Tags: threat, stride, spoofing, security-audit, adversarial]",
	}, t.Handle)
}

type ThreatModelerInput struct {
	models.UniversalPipelineInput
}

func (t *ThreatModelerTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ThreatModelerInput) (*mcp.CallToolResult, any, error) {
	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to load session: %v", err))
		return res, nil, nil
	}

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	var traceMap map[string]any
	if recallAvailable && session.ProjectRoot != "" {
		if tm, tmErr := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "go-refactor", session.ProjectRoot); tmErr == nil && tm != nil {
			traceMap = tm
		}
	}
	resp, analysisErr := t.Engine.AnalyzeThreatModel(ctx, input.Context, traceMap)
	if analysisErr != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("threat modeling failed: %v", analysisErr))
		return res, nil, nil
	}

	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["threat_model_narrative"] = resp.Data.Narrative

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("save session failed: %v", err))
		return res, nil, nil
	}

	if recallAvailable && session.ProjectRoot != "" {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, session.ProjectRoot, "threats_modeled", "native", "threat_model_auditor", "", session.Metadata)
	}

	var returnData any = resp
	var returnSummary string = resp.Summary

	if input.SessionID != "" && recallAvailable {
		saveErr := t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, resp)
		if saveErr == nil {
			returnSummary += fmt.Sprintf("\n[CSSA STATUS]: Complete STRIDE data saved successfully to recall session '%s'", input.SessionID)
			returnData = struct {
				Summary string `json:"summary"`
				Data    any    `json:"data,omitempty"`
			}{
				Summary: returnSummary,
				Data:    resp.Data,
			}
		} else {
			returnSummary += "\n[CSSA STATUS]: Could not save to recall. Falling back to standard JSON-RPC."
			resp.Summary = returnSummary
			returnData = resp
		}
	}

	return &mcp.CallToolResult{}, returnData, nil
}

// Register adds the security tools to the registry.
func Register(mgr *state.Manager, eng *engine.Engine) {
	registry.Global.Register(&ThreatModelerTool{Manager: mgr, Engine: eng})
}
