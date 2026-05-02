package safety

import (
	"context"
	"fmt"
	"go/ast"
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

// Tool implements the SQL injection guard tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_sql_injection_guard"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: THREAT] [PHASE: ADVERSARIAL] SQL INJECTION GUARD: Performs safety-focused AST analysis to detect unsafe dynamic database query and SQL string construction. Critical audit step to ensure refactoring has not introduced injection vectors. Produces vulnerability list with severity. [REQUIRES: brainstorm:threat_model_auditor] [Routing Tags: injection, sql-injection, safety, threat-vector, dynamic-sql]",
	}, t.Handle)
}

// Register adds the injection guard tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

type InjectionInput struct {
	models.UniversalPipelineInput
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input InjectionInput) (*mcp.CallToolResult, any, error) {
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
				session.Metadata["historical_injections"] = history
			}
		}
	}

	result, err := DetectInjections(ctx, input.Target)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	summary := "No potential SQL injection vectors detected."
	if len(result.Vulnerabilities) > 0 {
		summary = fmt.Sprintf("Found %d potential SQL injection vulnerabilities in %s", len(result.Vulnerabilities), input.Target)
	}

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			secStds := t.Engine.EnsureRecallCache(ctx, session, "security_standards", "search", map[string]any{"namespace": "ecosystem",
				"query": "SQL and NoSQL injection security AST standards for " + input.Target,
				"limit": 10,
			})
			session.Metadata["recall_cache_security"] = secStds

			if secStds != "" {
				summary += fmt.Sprintf("\n\n[Enterprise Security Standards]: %s", secStds)
			}
		}

		// Pillar metrics for brainstorm learning.
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":              "type_safety",
			"vulnerability_count": len(result.Vulnerabilities),
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)

		// AST faults.
		if len(result.Vulnerabilities) > 0 {
			var astFaults []string
			if f, ok := session.Metadata["ast_faults"].([]string); ok {
				astFaults = f
			}
			session.Metadata["ast_faults"] = append(astFaults, "sql_injection")
		}

		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "injection_scanned", "native", "go_sql_injection_guard", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string              `json:"summary"`
		Data    *SQLInjectionResult `json:"data"`
	}{
		Summary: summary,
		Data:    result,
	}, nil
}

// SQLInjectionResult details suspected injection vulnerabilities.
type SQLInjectionResult struct {
	Vulnerabilities []Vulnerability `json:"Vulnerabilities"`
}

// Vulnerability lists findings and their severity.
type Vulnerability struct {
	File     string `json:"File"`
	Line     int    `json:"Line"`
	Reason   string `json:"Reason"`
	Severity string `json:"Severity"`
	ASTTrace string `json:"ast_trace,omitempty"`
}

// DetectInjections analyzes SQL strings for dynamic concatenations.
func DetectInjections(ctx context.Context, pkgPath string) (*SQLInjectionResult, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	vulns := []Vulnerability{}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				switch ce := n.(type) {
				case *ast.CallExpr:
					if se, ok := ce.Fun.(*ast.SelectorExpr); ok {
						if isSQLFunction(se.Sel.Name) {
							for _, arg := range ce.Args {
								if isDirty(arg) {
									pos := pkg.Fset.Position(arg.Pos())
									vulns = append(vulns, Vulnerability{
										File:     pos.Filename,
										Line:     pos.Line,
										Reason:   fmt.Sprintf("Dynamic SQL concatenation in function call: %s", se.Sel.Name),
										Severity: "HIGH",
										ASTTrace: fmt.Sprintf(`{"node_type": "CallExpr", "trace": "%s", "dirty_arg": true}`, se.Sel.Name),
									})
								}
							}
						}
					}
				}
				return true
			})
		}
	}

	return &SQLInjectionResult{Vulnerabilities: vulns}, nil
}

func isSQLFunction(name string) bool {
	queries := []string{"Query", "QueryRow", "Exec", "QueryContext", "ExecContext"}
	for _, q := range queries {
		if strings.Contains(name, q) {
			return true
		}
	}
	return false
}

func isDirty(n ast.Node) bool {
	switch x := n.(type) {
	case *ast.BinaryExpr:
		if x.Op.String() == "+" {
			return true
		}
	case *ast.CallExpr:
		if se, ok := x.Fun.(*ast.SelectorExpr); ok {
			if se.Sel.Name == "Sprintf" {
				return true
			}
		}
	}
	return false
}
