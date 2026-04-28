package design

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

// AntithesisSkepticTool challenges thesis proposals for robustness.
type AntithesisSkepticTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

func (t *AntithesisSkepticTool) Name() string {
	return "antithesis_skeptic"
}

func (t *AntithesisSkepticTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: CRITIC] ANTITHESIS SKEPTIC: Stress-tests thesis proposals across 6 dimensions from a risk perspective (type safety overhead, modernization YAGNI, modularization fragmentation, etc). [TRIGGERS: brainstorm:aporia_engine] [Routing Tags: skeptic, devil-advocate, review-thesis, stress-test]",
	}, t.Handle)
}

// AntithesisInput is the input schema for the antithesis_skeptic tool.
type AntithesisInput struct {
	models.UniversalPipelineInput
}

func (t *AntithesisSkepticTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input AntithesisInput) (*mcp.CallToolResult, any, error) {
	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to load session: %v", err))
		return res, nil, nil
	}

	// Resolve standards.
	var standards string
	if stds, ok := session.Metadata["standards"].(string); ok && stds != "" {
		standards = stds
	}

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	// Build the thesis text from input context.
	// In orchestrator mode, also merge structured thesis from session handoff.
	thesisText := input.Context
	if isOrchestrator {
		if doc, ok := session.Metadata["thesis_document"].(models.ThesisDocument); ok {
			thesisText = thesisText + "\n\n" + doc.Data.Narrative
		}
	}

	// Orchestrator-only: load historical counter-thesis data.
	if recallAvailable && session.ProjectRoot != "" {
		if history := t.Engine.LoadCrossSessionFromRecall(ctx, "brainstorm", session.ProjectRoot); history != "" {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["historical_counter_thesis"] = history
		}
	}

	// Orchestrator-only: fetch live standards from recall.
	if standards == "" && recallAvailable {
		standards = t.Engine.EnsureRecallCache(ctx, session, "antithesis_skeptic", "search", map[string]interface{}{"namespace": "ecosystem", "query": "performance regression blast radius complexity", "domain": "robustness", "limit": 10})
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}
		session.Metadata["standards"] = standards
	}

	// Orchestrator-only: load go-refactor AST trace data.
	var traceMap map[string]interface{}
	if recallAvailable && session.ProjectRoot != "" {
		if tm, err := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "go-refactor", session.ProjectRoot); err == nil && tm != nil {
			traceMap = tm
		}
	}

	// Core engine call — works standalone.
	report, err := t.Engine.GenerateCounterThesis(ctx, thesisText, standards, traceMap)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	// Store counter-thesis in session.
	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["counter_thesis"] = report

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("save session failed: %v", err))
		return res, nil, nil
	}

	// Orchestrator-only: publish counter-thesis to recall.
	if recallAvailable && session.ProjectRoot != "" && session.Metadata != nil {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, session.ProjectRoot, "counter_thesis_generated", "native", "antithesis_skeptic", "", session.Metadata)
	}

	var returnData any = report
	var returnSummary string = report.Summary

	if input.SessionID != "" && recallAvailable {
		saveErr := t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, report)
		if saveErr == nil {
			returnSummary += fmt.Sprintf("\n[CSSA STATUS]: Complete structural data saved successfully to recall session '%s'", input.SessionID)
			returnData = struct {
				Summary string `json:"summary"`
				Data    any    `json:"data,omitempty"`
			}{
				Summary: returnSummary,
				Data:    report,
			}
		} else {
			returnSummary += "\n[CSSA STATUS]: Could not save to recall. Falling back to standard JSON-RPC."
			report.Summary = returnSummary
			returnData = report
		}
	}

	return &mcp.CallToolResult{}, returnData, nil
}
