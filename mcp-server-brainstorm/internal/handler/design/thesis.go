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

// ThesisArchitectTool proposes idiomatic Go 1.26.1 modernization.
type ThesisArchitectTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

func (t *ThesisArchitectTool) Name() string {
	return "thesis_architect"
}

func (t *ThesisArchitectTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: CRITIC] THESIS ARCHITECT: Evaluates Go codebases across 6 modernization dimensions. Proposes maximum Go 1.26.1 feature adoption. [TRIGGERS: brainstorm:antithesis_skeptic] [Routing Tags: thesis, propose, architect, 1.26.1, design]",
	}, t.Handle)
}

// ThesisInput is the input schema for the thesis_architect tool.
type ThesisInput struct {
	models.UniversalPipelineInput
}

func (t *ThesisArchitectTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ThesisInput) (*mcp.CallToolResult, any, error) {
	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to load session: %v", err))
		return res, nil, nil
	}

	// Resolve standards: session cache → recall (orchestrator only) → hardcoded default.
	var standards string
	if stds, ok := session.Metadata["standards"].(string); ok && stds != "" {
		standards = stds
	}

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	// Orchestrator-only: load historical thesis data from recall.
	if recallAvailable && session.ProjectRoot != "" {
		if history := t.Engine.LoadCrossSessionFromRecall(ctx, "brainstorm", session.ProjectRoot); history != "" {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["historical_thesis"] = history
		}
	}

	// Orchestrator-only: fetch live standards from recall.
	if standards == "" && recallAvailable {
		standards = t.Engine.EnsureRecallCache(ctx, session, "thesis_architect", "search", map[string]interface{}{"namespace": "ecosystem", "query": "Go 1.26.1 generics omitzero errors.AsType", "domain": "modernization", "limit": 10})
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

	// Core engine call — works standalone (traceMap == nil, standards from default).
	doc, err := t.Engine.GenerateThesis(ctx, input.Context, standards, traceMap)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	// Store thesis in session for antithesis_skeptic handoff.
	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["thesis_document"] = doc

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("save session failed: %v", err))
		return res, nil, nil
	}

	// Orchestrator-only: publish thesis to recall.
	if recallAvailable && session.ProjectRoot != "" && session.Metadata != nil {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, session.ProjectRoot, "thesis_generated", "native", "thesis_architect", "", session.Metadata)
	}

	var returnData any = doc
	var returnSummary string = doc.Summary

	if input.SessionID != "" && recallAvailable {
		saveErr := t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, doc)
		if saveErr == nil {
			returnSummary += fmt.Sprintf("\n[CSSA STATUS]: Complete structural data saved successfully to recall session '%s'", input.SessionID)
			returnData = struct {
				Summary string `json:"summary"`
				Data    any    `json:"data,omitempty"`
			}{
				Summary: returnSummary,
				Data:    doc,
			}
		} else {
			returnSummary += "\n[CSSA STATUS]: Could not save to recall. Falling back to standard JSON-RPC."
			doc.Summary = returnSummary
			returnData = doc
		}
	}

	return &mcp.CallToolResult{}, returnData, nil
}
