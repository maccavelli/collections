package metrics

import (
	"context"
	"fmt"
	"go/ast"
	"log/slog"
	"os"

	"golang.org/x/tools/go/ast/inspector"

	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"

	"github.com/tidwall/buntdb"

	"github.com/fzipp/gocyclo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uudashr/gocognit"
)

// Tool implements the complexity analyzer tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_complexity_analyzer"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] COMPLEXITY SCORER: Calculates cyclomatic and cognitive complexity for functions by walking the control-flow graph (CFG). Reports branch hotspots exceeding configurable scoring thresholds. Produces per-function cyclomatic/cognitive scores. In orchestrator mode, enriches with recall standards and ADR exemptions.",
	}, t.Handle)
}

// Register adds the complexity analyzer tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

type ComplexityInput struct {
	models.UniversalPipelineInput
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ComplexityInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	// Stage 1: Load session and query standards BEFORE analysis
	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Target)

		if recallAvailable {
			// Query Recall securely via WaitGroup synchronization for baseline complexity standards
			standards := t.Engine.EnsureRecallCache(ctx, session, "metrics_complexity", "search", map[string]any{"namespace": "ecosystem", "query": "Go cyclomatic and cognitive complexity thresholds", "limit": 10})
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["recall_cache_metrics"] = standards

			// Phase 1: Load cross-server context from brainstorm if available.
			if peerData := t.Engine.LoadCrossSessionFromRecall(ctx, "brainstorm", input.Target); peerData != "" {
				session.Metadata["peer_context_brainstorm"] = peerData
			}

			// Phase 2: Load historical complexity metrics for Delta-tracking pass-throughs.
			if history := t.Engine.LoadCrossSessionFromRecall(ctx, "gorefactor", input.Target); history != "" {
				session.Metadata["historical_complexity"] = history
			}

			// Phase 3: Query Recall for intentional Architectural Decision Records (ADR) exemptions.
			adrs := t.Engine.EnsureRecallCache(ctx, session, "adr_exemptions", "search", map[string]any{"namespace": "ecosystem", "query": "ADR Exemptions and intentional legacy overrides", "package": input.Target, "limit": 5})
			if adrs != "" {
				session.Metadata["recall_adrs"] = adrs
			}
		}
	}

	var db *buntdb.DB
	if t.Engine != nil {
		db = t.Engine.DB
	}

	result, err := CalculateComplexity(ctx, db, input.Target)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	summary := fmt.Sprintf("Complexity analysis for %s: %d functions analyzed", input.Target, len(result.Functions))

	// ---- CSSA / Pagination Fallback Logic ----
	var returnData any = result

	if recallAvailable && input.SessionID != "" {
		// Attempt to save to Recall
		saveErr := t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, result)
		if saveErr == nil {
			summary += fmt.Sprintf("\n[CSSA STATUS]: Complete structural data saved successfully to recall session '%s'", input.SessionID)
			returnData = nil // Do not echo massive data across stdio
		} else {
			summary += "\n[CSSA STATUS]: Could not save to recall. Falling back to JSON-RPC pagination."
		}
	}

	limit := 500
	if lim, ok := input.Flags["limit"].(float64); ok {
		limit = int(lim)
	}
	offset := 0
	if off, ok := input.Flags["offset"].(float64); ok {
		offset = int(off)
	}

	if returnData != nil {
		p := util.Pagination{SessionID: input.SessionID, Limit: limit, Offset: offset}
		start, end := p.Apply(len(result.Functions))
		if len(result.Functions) > (end - start) {
			summary += fmt.Sprintf("\n[PAGINATION WARNING]: Payload truncated. Showing items %d-%d out of %d total. Pass a higher limit/offset if necessary.", start, end, len(result.Functions))
			truncated := make(map[string]FunctionMetrics)
			i := 0
			for k, v := range result.Functions {
				if i >= start && i < end {
					truncated[k] = v
				}
				i++
			}
			returnData = &MetricResult{Functions: truncated}
		}
	}
	// ------------------------------------------

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		// Pillar metrics for brainstorm learning.
		totalFuncs := len(result.Functions)
		maxCyclomatic := 0
		maxCognitive := 0
		for _, fm := range result.Functions {
			if fm.Cyclomatic > maxCyclomatic {
				maxCyclomatic = fm.Cyclomatic
			}
			if fm.Cognitive > maxCognitive {
				maxCognitive = fm.Cognitive
			}
		}
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":          "efficiency",
			"total_functions": totalFuncs,
			"max_cyclomatic":  maxCyclomatic,
			"max_cognitive":   maxCognitive,
		}

		// AST faults.
		if maxCyclomatic > 15 {
			var astFaults []string
			if f, ok := session.Metadata["ast_faults"].([]string); ok {
				astFaults = f
			}
			session.Metadata["ast_faults"] = append(astFaults, "high_complexity")
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "complexity_analyzed", "native", "go_complexity_analyzer", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string `json:"summary"`
		Data    any    `json:"data,omitempty"`
	}{
		Summary: summary,
		Data:    returnData,
	}, nil
}

// FunctionMetrics stores the calculated complexity scores for a function.
type FunctionMetrics struct {
	Cyclomatic int `json:"cyclomatic"`
	Cognitive  int `json:"cognitive"`
}

// MetricResult contains complexity scores for all functions in the package.
type MetricResult struct {
	Functions map[string]FunctionMetrics `json:"Functions"`
}

// CalculateComplexity runs cyclomatic and cognitive complexity analysis.
func CalculateComplexity(ctx context.Context, db *buntdb.DB, pkgPath string) (*MetricResult, error) {
	res, err := loader.Discover(ctx, pkgPath)
	if err != nil {
		return nil, err
	}

	cacheKey := "complexity:" + pkgPath
	return loader.CachedEvaluate(db, cacheKey, res.Workspace.ModuleRoot, func() (*MetricResult, error) {
		pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.SyntaxMode)
		if err != nil {
			return nil, err
		}

		funcs := make(map[string]FunctionMetrics)

		for _, pkg := range pkgs {
			for _, file := range pkg.Syntax {
				insp := inspector.New([]*ast.File{file})
				insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
					f := n.(*ast.FuncDecl)
					name := f.Name.Name
					if f.Recv != nil {
						// Method name formatting
						typeName := "unknown"
						if len(f.Recv.List) > 0 {
							switch t := f.Recv.List[0].Type.(type) {
							case *ast.Ident:
								typeName = t.Name
							case *ast.StarExpr:
								if id, ok := t.X.(*ast.Ident); ok {
									typeName = "*" + id.Name
								}
							}
						}
						name = fmt.Sprintf("(%s).%s", typeName, name)
					}

					funcs[name] = FunctionMetrics{
						Cyclomatic: gocyclo.Complexity(f),
						Cognitive:  gocognit.Complexity(f),
					}
				})
			}
		}

		return &MetricResult{
			Functions: funcs,
		}, nil
	})
}
