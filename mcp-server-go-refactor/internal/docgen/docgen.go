package docgen

import (
	"context"
	"fmt"
	"go/ast"
	"log/slog"
	"os"

	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool implements the doc generator tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_doc_generator"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] DOCUMENTATION AUDITOR: Audits the codebase for missing comments, and writes compliant docs substituting undocumented public API surface with generated godoc suggestions. Produces undocumented symbol list.",
	}, t.Handle)
}

// Register adds the doc generator tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

type DocInput struct {
	models.UniversalPipelineInput
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DocInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Target)
	}

	docs, err := GenerateDocs(ctx, input.Target)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	summary := "All exported symbols have documentation."
	if len(docs.MissingComments) > 0 {
		summary = fmt.Sprintf("Found %d undocumented exported symbols in %s", len(docs.MissingComments), input.Target)
	}

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			godocStds := t.Engine.EnsureRecallCache(ctx, session, "godoc_standards", "search", map[string]any{"namespace": "ecosystem",
				"query":       "godoc formatting conventions standard",
				"package":     input.Target,
				"symbol_type": "func",
				"limit":       10,
			})
			session.Metadata["recall_cache_godoc"] = godocStds

			if godocStds != "" {
				summary += fmt.Sprintf("\n\n[Godoc Documentation Standards]: %s", godocStds)
			}
		}

		// Pillar metrics for brainstorm learning.
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":             "maintainability",
			"undocumented_count": len(docs.MissingComments),
		}

		// AST faults.
		if len(docs.MissingComments) > 0 {
			var astFaults []string
			if f, ok := session.Metadata["ast_faults"].([]string); ok {
				astFaults = f
			}
			session.Metadata["ast_faults"] = append(astFaults, "missing_documentation")
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "docs_generated", "native", "go_doc_generator", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string      `json:"summary"`
		Data    *DocSummary `json:"data"`
	}{
		Summary: summary,
		Data:    docs,
	}, nil
}

// DocSummary contains identified public definitions lacking comments.
type DocSummary struct {
	MissingComments []string `json:"MissingComments"`
}

// GenerateDocs identifies exported methods, structs, and variables without comments.
func GenerateDocs(ctx context.Context, pkgPath string) (*DocSummary, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	missing := []string{}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				switch decl := n.(type) {
				case *ast.FuncDecl:
					if ast.IsExported(decl.Name.Name) && decl.Doc == nil {
						missing = append(missing, fmt.Sprintf("func %s (file: %s)", decl.Name.Name, pkg.Fset.Position(decl.Pos()).Filename))
					}
				case *ast.GenDecl:
					for _, spec := range decl.Specs {
						if ts, ok := spec.(*ast.TypeSpec); ok {
							if ast.IsExported(ts.Name.Name) && decl.Doc == nil {
								missing = append(missing, fmt.Sprintf("type %s (file: %s)", ts.Name.Name, pkg.Fset.Position(decl.Pos()).Filename))
							}
						}
					}
				}
				return true
			})
		}
	}

	return &DocSummary{
		MissingComments: missing,
	}, nil
}
