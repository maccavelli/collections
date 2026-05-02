package design

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
)

// PeerReviewTool exposes a multi-agent overlay mechanism.
type PeerReviewTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

func (t *PeerReviewTool) Name() string {
	return "peer_review"
}

func (t *PeerReviewTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: CRITIC] PEER REVIEW ENGINE: Performs multi-perspective review of component designs, dead code removal candidates, and interface extraction proposals. Validates that structural changes identified by go-refactor won't cause regressions. [REQUIRES: go-refactor:go_dead_code_pruner, go-refactor:go_interface_discovery] [Routing Tags: peer-review, heuristic-check, design-evaluation, validate-removal]",
	}, t.Handle)
}

type PeerReviewInput struct {
	models.UniversalPipelineInput
	Focus           string `json:"focus"`
	ComponentDesign string `json:"component_design"`
}

func (t *PeerReviewTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input PeerReviewInput) (*mcp.CallToolResult, any, error) {
	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to load session: %v", err))
		return res, nil, nil
	}

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	// Load standards from session.
	var standards string
	if stds, ok := session.Metadata["standards"].(string); ok {
		standards = stds
	}

	// TraceMap enrichment.
	var traceMap map[string]any
	if recallAvailable && session.ProjectRoot != "" {
		if tm, tmErr := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "go-refactor", session.ProjectRoot); tmErr == nil && tm != nil {
			traceMap = tm
		}
	}

	var response struct {
		Verdict   string `json:"verdict"`
		Review    string `json:"review"`
		Telemetry string `json:"telemetry"`
	}

	if recallAvailable {
		// Orchestrator path: dispatch to specialized agents via proxy.
		timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		args := map[string]any{
			"target":           input.Target,
			"session_id":       input.SessionID,
			"focus":            input.Focus,
			"component_design": input.ComponentDesign,
		}

		resultData := t.Engine.ExternalClient.CallDatabaseTool(timeoutCtx, "run_pipeline", args)

		if resultData == "" {
			if timeoutCtx.Err() == context.DeadlineExceeded {
				slog.Warn("[PEER_REVIEW] orchestrator cascade timeout, falling back to standalone")
			}
			// Fall through to standalone path below.
		} else {
			response.Verdict = "completed"
			response.Review = resultData
			response.Telemetry = fmt.Sprintf("Dispatched to %s agent", input.Focus)
		}
	}

	// Standalone fallback: run local CritiqueDesign.
	if response.Review == "" {
		critiqueResp, critiqueErr := t.Engine.CritiqueDesign(ctx, input.ComponentDesign, standards, traceMap)
		if critiqueErr != nil {
			res := &mcp.CallToolResult{}
			res.SetError(fmt.Errorf("standalone peer review failed: %v", critiqueErr))
			return res, nil, nil
		}
		response.Verdict = "completed"
		response.Review = critiqueResp.Data.Narrative
		response.Telemetry = fmt.Sprintf("Standalone %s review via local CritiqueDesign", input.Focus)
	}

	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["last_peer_review"] = response

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to save session: %v", err))
		return res, nil, nil
	}

	// Publish peer review trace to recall.
	if recallAvailable && session.ProjectRoot != "" {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, session.ProjectRoot, "peer_reviewed", "native", "peer_review", "", session.Metadata)
	}

	return &mcp.CallToolResult{}, response, nil
}
