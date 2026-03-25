package docgen

import (
	"context"
	"fmt"
	"go/ast"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool implements the doc generator tool.
type Tool struct{}

func (t *Tool) Name() string {
	return "go_doc_generator"
}

func (t *Tool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "QUALITY MANDATE / DOC AUDIT: Audits the codebase for exported types, functions, and variables that lack Go documentation comments. Call this as a final quality gate to ensure API accessibility. Cascades to go_test_coverage_tracer.",
	}, t.Handle)
}

// Register adds the doc generator tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

type DocInput struct {
	Pkg string `json:"pkg" jsonschema:"The package path to audit"`
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DocInput) (*mcp.CallToolResult, any, error) {
	docs, err := GenerateDocs(ctx, input.Pkg)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%+v", docs)}},
	}, nil, nil
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

