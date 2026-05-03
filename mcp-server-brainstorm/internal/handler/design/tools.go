// Package design provides functionality for the design subsystem.
package design

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

// CritiqueDesignTool handles design assessment.
type CritiqueDesignTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

// Name performs the Name operation.
func (t *CritiqueDesignTool) Name() string {
	return "critique_design"
}

// Register performs the Register operation.
func (t *CritiqueDesignTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: CRITIC] DESIGN CRITIQUE ENGINE: Subjects architectural designs, feature plans, and major modifications to a rigorous, multi-perspective review. Evaluates quality metrics and generates red-team challenges based on go-refactor memory and context analysis findings. [REQUIRES: go-refactor:go_memory_analyzer, go-refactor:go_context_analyzer] [Routing Tags: critique, review, red-team, design-check, evaluate-plan]",
	}, t.Handle)
}

// DesignInput defines the DesignInput structure.
type DesignInput struct {
	models.UniversalPipelineInput
}

// Handle performs the Handle operation.
func (t *CritiqueDesignTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DesignInput) (*mcp.CallToolResult, any, error) {
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

	var goldenTruth string
	if standards, ok := session.Metadata["standards"].(string); ok && standards != "" {
		goldenTruth = standards
	}

	if recallAvailable && session.ProjectRoot != "" {
		if critHistory := t.Engine.LoadCrossSessionFromRecall(ctx, "brainstorm", session.ProjectRoot); critHistory != "" {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["historical_critiques"] = critHistory
		}
	}

	if goldenTruth == "" && recallAvailable {
		goldenTruth = t.Engine.EnsureRecallCache(ctx, session, "critique_design", "search", map[string]any{"namespace": "ecosystem", "query": "anti-patterns edge cases", "domain": "design", "limit": 10})
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}
		session.Metadata["standards"] = goldenTruth
	}

	var traceMap map[string]any
	if recallAvailable && session.ProjectRoot != "" {
		if tm, err := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "go-refactor", session.ProjectRoot); err == nil && tm != nil {
			traceMap = tm
		}
	}

	resp, err := t.Engine.CritiqueDesign(ctx, input.Context, goldenTruth, traceMap)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["critique_findings"] = []string{resp.Data.Narrative}

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("save session failed: %v", err))
		return res, nil, nil
	}

	if recallAvailable && session.ProjectRoot != "" {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, session.ProjectRoot, "critique_completed", "native", "critique_design", "", session.Metadata)
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
				Data:    nil,
			}
		} else {
			returnSummary += "\n[CSSA STATUS]: Could not save to recall. Falling back to standard JSON-RPC."
			resp.Summary = returnSummary
			returnData = resp
		}
	}

	return &mcp.CallToolResult{}, returnData, nil
}

// AnalyzeEvolutionTool handles risk identification in changes.
type AnalyzeEvolutionTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

// Name performs the Name operation.
func (t *AnalyzeEvolutionTool) Name() string {
	return "analyze_evolution"
}

// Register performs the Register operation.
func (t *AnalyzeEvolutionTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: CRITIC] EVOLUTION RISK ANALYZER: Evaluates and predicts the potential cascading risks and future blast radius of proposed architectural changes or refactors. Categorizes change impact and assesses risk level. [REQUIRES: go-refactor:go_ast_suite_analyzer] [Routing Tags: risk, impact, blast-radius, evolution, analyze-future]",
	}, t.Handle)
}

// EvolutionInput defines the EvolutionInput structure.
type EvolutionInput struct {
	models.UniversalPipelineInput
}

// Handle performs the Handle operation.
func (t *AnalyzeEvolutionTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input EvolutionInput) (*mcp.CallToolResult, any, error) {
	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("load session: %v", err))
		return res, nil, nil
	}

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	var standards string
	if stds, ok := session.Metadata["standards"].(string); ok {
		standards = stds
	}

	if recallAvailable && session.ProjectRoot != "" {
		if evoHistory := t.Engine.LoadCrossSessionFromRecall(ctx, "brainstorm", session.ProjectRoot); evoHistory != "" {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["historical_evolution"] = evoHistory
		}
	}

	if standards == "" && recallAvailable {
		standards = t.Engine.EnsureRecallCache(ctx, session, "analyze_evolution", "search", map[string]any{"namespace": "ecosystem", "query": "technical debt blast radius compatibility", "domain": "evolution", "limit": 10})
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}
		session.Metadata["standards"] = standards
	}

	var traceMap map[string]any
	if recallAvailable && session.ProjectRoot != "" {
		if tm, err := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "go-refactor", session.ProjectRoot); err == nil && tm != nil {
			traceMap = tm
		}
	}

	result, err := t.Engine.AnalyzeEvolution(ctx, input.Context, standards, traceMap)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["evolution_findings"] = []string{result.Data.Narrative}

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("save session failed: %v", err))
		return res, nil, nil
	}

	if recallAvailable && session.ProjectRoot != "" {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, session.ProjectRoot, "evolution_analyzed", "native", "analyze_evolution", "", session.Metadata)
	}

	var returnData any = result
	var returnSummary string = result.Summary

	if input.SessionID != "" && recallAvailable {
		saveErr := t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, result)
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
			result.Summary = returnSummary
			returnData = result
		}
	}

	return &mcp.CallToolResult{}, returnData, nil
}

// ClarifyRequirementsTool handles requirement grounding.
type ClarifyRequirementsTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

// Name performs the Name operation.
func (t *ClarifyRequirementsTool) Name() string {
	return "clarify_requirements"
}

// Register performs the Register operation.
func (t *ClarifyRequirementsTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] [PHASE: BOOTSTRAP] REQUIREMENTS CLARIFIER: Analyzes feature requirements to detect architectural ambiguity before design. Identifies decision forks and provides Socratic prompts. Populates session with requirements_findings. [Routing Tags: requirements, clarify, questions, specifications, ambiguity]",
	}, t.Handle)
}

// RequirementsInput defines the RequirementsInput structure.
type RequirementsInput struct {
	models.UniversalPipelineInput
}

// Handle performs the Handle operation.
func (t *ClarifyRequirementsTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input RequirementsInput) (*mcp.CallToolResult, any, error) {
	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("load session: %v", err))
		return res, nil, nil
	}

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	var standards string
	if stds, ok := session.Metadata["standards"].(string); ok {
		standards = stds
	}

	if recallAvailable && session.ProjectRoot != "" {
		if reqHistory := t.Engine.LoadCrossSessionFromRecall(ctx, "brainstorm", session.ProjectRoot); reqHistory != "" {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["historical_requirements"] = reqHistory
		}
	}

	if standards == "" && recallAvailable {
		standards = t.Engine.EnsureRecallCache(ctx, session, "clarify_requirements", "search", map[string]any{"namespace": "ecosystem", "query": "architectural ambiguity requirements constraints", "domain": "architecture", "limit": 10})
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}
		session.Metadata["standards"] = standards
	}

	// TraceMap enrichment: ground requirements in actual code structure.
	if recallAvailable && session.ProjectRoot != "" {
		if tm, err := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "go-refactor", session.ProjectRoot); err == nil && tm != nil {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["go_refactor_trace"] = tm
		}
	}

	resp, err := t.Engine.ClarifyRequirements(ctx, input.Context, standards)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["requirements_findings"] = []string{resp.Data.Narrative}

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("save session failed: %v", err))
		return res, nil, nil
	}

	if recallAvailable && session.ProjectRoot != "" {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, session.ProjectRoot, "requirements_clarified", "native", "clarify_requirements", "", session.Metadata)
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
				Data:    nil,
			}
		} else {
			returnSummary += "\n[CSSA STATUS]: Could not save to recall. Falling back to standard JSON-RPC."
			resp.Summary = returnSummary
			returnData = resp
		}
	}

	return &mcp.CallToolResult{}, returnData, nil
}

// Register adds the design tools to the registry.
func Register(mgr *state.Manager, eng *engine.Engine) {
	registry.Global.Register(&CritiqueDesignTool{Manager: mgr, Engine: eng})
	registry.Global.Register(&AnalyzeEvolutionTool{Manager: mgr, Engine: eng})
	registry.Global.Register(&ClarifyRequirementsTool{Manager: mgr, Engine: eng})
	registry.Global.Register(&ArchitecturalDiagrammerTool{Manager: mgr, Engine: eng})
	registry.Global.Register(&PeerReviewTool{Manager: mgr, Engine: eng})
	registry.Global.Register(&ThesisArchitectTool{Manager: mgr, Engine: eng})
	registry.Global.Register(&AntithesisSkepticTool{Manager: mgr, Engine: eng})
}
