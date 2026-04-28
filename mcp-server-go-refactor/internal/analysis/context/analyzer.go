package contextanalysis

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"log/slog"
	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool implements the context propagation analyzer tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_context_analyzer"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] CONTEXT PROPAGATION AUDITOR: Audits call chains to ensure robust propagation, identifying missing ctxs where parent contexts are dropped. Detects broken async patterns and context forwarding in goroutine launches. Produces context propagation violation list. [Routing Tags: context, context.Context, propagate, async-patterns, timeouts, cancel-propagation]",
	}, t.Handle)
}

// Register adds the context propagation analyzer tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

type ContextInput struct {
	models.UniversalPipelineInput
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ContextInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Target)

		if recallAvailable {
			if history := t.Engine.LoadCrossSessionFromRecall(ctx, "gorefactor", input.Target); history != "" {
				if session.Metadata == nil {
					session.Metadata = make(map[string]any)
				}
				session.Metadata["historical_context"] = history
			}
		}
	}

	findings, err := AnalyzeContext(ctx, input.Target)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	summary := "No context propagation issues found."
	if len(findings) > 0 {
		summary = fmt.Sprintf("Found %d context propagation issues in package %s", len(findings), input.Target)
	}

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			ctxStds := t.Engine.EnsureRecallCache(ctx, session, "context_propagation", "search", map[string]interface{}{"namespace": "ecosystem",
				"query": "Go context propagation standards, cancellation patterns, timeout policy conventions, and distributed tracing standards for " + input.Target,
				"limit": 15,
			})
			session.Metadata["recall_cache_context"] = ctxStds

			if ctxStds != "" {
				summary += fmt.Sprintf("\n\n[Context Propagation Standards]: %s", ctxStds)
			}
		}

		// Pillar metrics for brainstorm learning.
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":                  "reliability",
			"context_violation_count": len(findings),
		}

		// AST faults.
		if len(findings) > 0 {
			var astFaults []string
			if f, ok := session.Metadata["ast_faults"].([]string); ok {
				astFaults = f
			}
			session.Metadata["ast_faults"] = append(astFaults, "context_violation")
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "context_analyzed", "native", "go_context_analyzer", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string    `json:"summary"`
		Data    []Finding `json:"data"`
	}{
		Summary: summary,
		Data:    findings,
	}, nil
}

type Finding struct {
	File      string `json:"File"`
	Line      int    `json:"Line"`
	Function  string `json:"Function"`
	Rationale string `json:"Rationale"`
}

func AnalyzeContext(ctx context.Context, pkgPath string) ([]Finding, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	findings := []Finding{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				// Check if calling a function that takes context as first param
				// but passing context.Background() or context.TODO()
				if isContextDeprivedCall(pkg.TypesInfo, call) {
					pos := pkg.Fset.Position(call.Pos())
					findings = append(findings, Finding{
						File:      pos.Filename,
						Line:      pos.Line,
						Function:  fmt.Sprintf("%v", call.Fun),
						Rationale: "Call ignores potential parent context by using context.Background() or context.TODO().",
					})
				}
				return true
			})
		}
	}
	return findings, nil
}

func isContextDeprivedCall(info *types.Info, call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}

	// 1. Check if the first argument is specifically context.Background() or context.TODO()
	firstArg := call.Args[0]
	argCall, ok := firstArg.(*ast.CallExpr)
	if !ok {
		return false
	}

	sel, ok := argCall.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	if sel.Sel.Name != "Background" && sel.Sel.Name != "TODO" {
		return false
	}

	// Verify it's the "context" package
	if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "context" {
		return false
	}

	// 2. STRENGTHEN: Check if the function being called actually expects context.Context as first argument.
	// This uses type info to avoid false positives.
	var funObj types.Object
	if info != nil {
		switch f := call.Fun.(type) {
		case *ast.Ident:
			funObj = info.Uses[f]
		case *ast.SelectorExpr:
			funObj = info.Uses[f.Sel]
		}
	}

	if funObj == nil {
		return true // Fallback to naive if type info missing
	}

	sig, ok := funObj.Type().Underlying().(*types.Signature)
	if !ok || sig.Params().Len() == 0 {
		return false
	}

	firstParam := sig.Params().At(0)
	paramType := firstParam.Type().String()

	// Check if param type is context.Context
	return strings.Contains(paramType, "context.Context")
}
