package astsuite

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"mcp-server-go-refactor/internal/dependency"
	"mcp-server-go-refactor/internal/docgen"
	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/layout"
	"mcp-server-go-refactor/internal/metrics"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/modernizer"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
)

// Tool implements the AST suite macro-analyzer tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_ast_suite_analyzer"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] AST SUITE ANALYZER: Comprehensive structural diagnostic suite parsing Go AST bounds. Executes recursive sweeps tracking cyclomatic complexity limits, missing contextual propagation, dependency impact footprints, deprecated logic modernization scopes, and undocumented topologies. [TRIGGERS: brainstorm:brainstorm_complexity_forecaster, brainstorm:analyze_evolution] [Routing Tags: ast-scan, inspect, audit-code, structural-diagnostic, complexity-sweep]",
	}, t.Handle)
}

// Register adds the AST suite tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

type ASTSuiteInput struct {
	models.UniversalPipelineInput
}

type ASTSuiteResult struct {
	Modernization []modernizer.Finding    `json:"Modernization"`
	Complexity    *metrics.MetricResult   `json:"Complexity"`
	Documentation *docgen.DocSummary      `json:"Documentation"`
	Dependency    *dependency.Impact      `json:"Dependency"`
	Alignment     *layout.AlignmentResult `json:"Alignment,omitempty"`
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ASTSuiteInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Target)
	}

	var db *buntdb.DB
	if t.Engine != nil {
		db = t.Engine.DB
	}

	result := &ASTSuiteResult{}
	var summary string

	astHash := engine.ComputeASTHash(input.Target)
	cacheKey := fmt.Sprintf("ast_cache:%s", astHash)

	if db != nil {
		var cachedRaw string
		_ = db.View(func(tx *buntdb.Tx) error {
			val, err := tx.Get(cacheKey)
			if err == nil {
				cachedRaw = val
			}
			return err
		})
		if cachedRaw != "" {
			if err := json.Unmarshal([]byte(cachedRaw), result); err == nil {
				summary = fmt.Sprintf("[CACHE HIT] Recovered structural metrics instantly for hash: %s", astHash[:8])
				goto FINALIZE
			}
		}
	}

	// 1. Complexity
	if cRes, cErr := metrics.CalculateComplexity(ctx, db, input.Target); cErr == nil && cRes != nil {
		result.Complexity = cRes
		summary += fmt.Sprintf("Calculated complexity metrics for %d functions. ", len(cRes.Functions))
	}

	// 2. Modernization
	if mRes, mErr := modernizer.Analyze(ctx, input.Target); mErr == nil {
		result.Modernization = mRes
		summary += fmt.Sprintf("Identified %d modernization opportunities. ", len(mRes))
	}

	// 3. Documentation
	if dRes, dErr := docgen.GenerateDocs(ctx, input.Target); dErr == nil && dRes != nil {
		result.Documentation = dRes
		summary += fmt.Sprintf("Found %d undocumented exported symbols. ", len(dRes.MissingComments))
	}

	// 4. Dependencies
	if depRes, depErr := dependency.Analyze(ctx, input.Target); depErr == nil && depRes != nil {
		result.Dependency = depRes
		summary += fmt.Sprintf("Discovered %d transitive dependencies. ", len(depRes.Modules))
	}

	// 5. Layout / Cycler
	if input.Context != "" {
		if lRes, lErr := layout.AnalyzeStructAlignment(ctx, input.Context, input.Target); lErr == nil && lRes != nil {
			result.Alignment = lRes
			summary += fmt.Sprintf("Analyzed alignment mapping for struct %s. ", input.Context)
		}
	}

	// Cache result
	if db != nil {
		if b, err := json.Marshal(result); err == nil {
			_ = db.Update(func(tx *buntdb.Tx) error {
				_, _, err := tx.Set(cacheKey, string(b), nil)
				return err
			})
		}
	}

FINALIZE:

	if summary == "" {
		summary = "AST suite executed, but no structural metrics were mapped locally."
	}

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		if result.Complexity != nil {
			session.Metadata["complexity_metrics"] = result.Complexity
		}
		if result.Modernization != nil {
			session.Metadata["modernization_sweep"] = result.Modernization
		}
		if result.Documentation != nil {
			session.Metadata["documentation_metrics"] = result.Documentation
		}
		if result.Dependency != nil {
			session.Metadata["dependency_metrics"] = result.Dependency
		}
		if result.Alignment != nil {
			session.Metadata["alignment_metrics"] = result.Alignment
		}
		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "ast_suite_analyzed", "native", "go_ast_suite_analyzer", "", session.Metadata)
		}
	}

	if input.SessionID != "" && recallAvailable {
		_ = t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, result)
	}

	return &mcp.CallToolResult{}, struct {
		Summary string          `json:"summary"`
		Data    *ASTSuiteResult `json:"data"`
	}{
		Summary: summary,
		Data:    result,
	}, nil
}
