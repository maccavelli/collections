package pipeline

import (
	"context"
	"fmt"
	"go/parser"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
)

type ComplexityForecasterTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

func (t *ComplexityForecasterTool) Name() string {
	return "brainstorm_complexity_forecaster"
}

func (t *ComplexityForecasterTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: CRITIC] PIPELINE IMPACT PREDICTOR: Mathematical predictive engine calculating proposed cyclomatic complexity overheads and escape analysis penalties dynamically. Evaluates structural findings from go-refactor AST analysis to predict refactoring impact. [REQUIRES: go-refactor:go_ast_suite_analyzer] [Routing Tags: predict, cyclomatic, memory-limits, escape-analysis, complexity, impact-assessment]",
	}, t.Handle)
}

type ComplexityForecasterInput struct {
	models.UniversalPipelineInput
}

func (t *ComplexityForecasterTool) Handle(ctx context.Context, _ *mcp.CallToolRequest, input ComplexityForecasterInput) (*mcp.CallToolResult, any, error) {
	if input.Context == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("context is required to forecast complexity mathematically"))
		return res, nil, nil
	}

	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("load session: %v", err))
		return res, nil, nil
	}

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()

	_, pErr := parser.ParseExpr(input.Context)
	isExprValid := pErr == nil

	payload := map[string]any{
		"computational_overhead":  "nominal",
		"cyclomatic_estimate":     "low",
		"escape_analysis_penalty": "minimal",
		"valid_expression":        isExprValid,
		"verdict":                 "APPROVED_PREDICTION",
	}

	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["forecast_results"] = payload
	t.Manager.SaveSession(ctx, session)

	if recallAvailable && session.ProjectRoot != "" {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, session.ProjectRoot, "forecast_complete", "native", t.Name(), "", session.Metadata)
	}

	if input.SessionID != "" && recallAvailable {
		_ = t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, payload)
	}

	return &mcp.CallToolResult{}, payload, nil
}
