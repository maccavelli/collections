// Package discovery provides functionality for the discovery subsystem.
package discovery

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

// DiscoverProjectTool handles the unified discovery scan.
type DiscoverProjectTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

// Name performs the Name operation.
func (t *DiscoverProjectTool) Name() string {
	return "discover_project"
}

// Register performs the Register operation.
func (t *DiscoverProjectTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] [PHASE: BOOTSTRAP] PROJECT DISCOVERY: Scans and audits target repositories for comprehensive structural analysis. Identifies technical debt, missing components, gaps, and integration points. [REQUIRES: Initial feature ambiguity clarification step] [PIPELINE CONSTRAINT: Do not invoke autonomously. Must operate via strict pipeline coordinates.] [Routing Tags: audit, discover, repository-scan, analyze-codebase]",
	}, t.Handle)
}

// DiscoverInput defines the DiscoverInput structure.
type DiscoverInput struct {
	models.UniversalPipelineInput
}

// Handle performs the Handle operation.
func (t *DiscoverProjectTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DiscoverInput) (*mcp.CallToolResult, any, error) {
	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("load session: %v", err))
		return res, nil, nil
	}

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	// Phase 1: Load cross-server context from go-refactor if available.
	if recallAvailable {
		if peerData := t.Engine.LoadCrossSessionFromRecall(ctx, "gorefactor", t.Engine.ResolvePath(input.Target)); peerData != "" {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["peer_context_gorefactor"] = peerData
		}
	}

	// Phase 2: Load own historical discoveries for trend analysis.
	if recallAvailable {
		if selfHistory := t.Engine.LoadCrossSessionFromRecall(ctx, "brainstorm", t.Engine.ResolvePath(input.Target)); selfHistory != "" {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["historical_discoveries"] = selfHistory
		}
	}

	// Phase 3: TraceMap enrichment from go-refactor AST data.
	if recallAvailable {
		projectPath := t.Engine.ResolvePath(input.Target)
		if projectPath != "" {
			if tm, tmErr := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "go-refactor", projectPath); tmErr == nil && tm != nil {
				if session.Metadata == nil {
					session.Metadata = make(map[string]any)
				}
				session.Metadata["go_refactor_trace"] = tm
			}
		}
	}

	resp, err := t.Engine.DiscoverProject(ctx, input.Target, session)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("save session: %v", err))
		return res, nil, nil
	}

	projectID := t.Engine.ResolvePath(input.Target)
	if projectID == "" {
		projectID = session.ProjectRoot
	}
	if recallAvailable && projectID != "" && session.Metadata != nil {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, projectID, "discovered", "native", "discover_project", "", session.Metadata)
	}

	var returnData any = resp
	var returnSummary string = resp.Summary

	if input.SessionID != "" && recallAvailable {
		saveErr := t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, resp)
		if saveErr == nil {
			returnSummary += fmt.Sprintf("\n[CSSA STATUS]: Complete structural data saved successfully to recall session '%s'", input.SessionID)
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

// Register adds the discovery tools to the registry.
func Register(mgr *state.Manager, eng *engine.Engine) {
	registry.Global.Register(&DiscoverProjectTool{
		Manager: mgr,
		Engine:  eng,
	})
}
