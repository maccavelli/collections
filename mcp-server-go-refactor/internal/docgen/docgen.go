package docgen

import (
	"context"
	"fmt"
	"go/ast"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tool implements the doc generator tool.
type Tool struct{}

// Register adds the doc generator tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_doc_generator",
		mcp.WithDescription("Audits the codebase for exported types, functions, and variables that lack proper Go documentation comments. Use this as a quality gate to ensure that the public API of a package remains well-documented and accessible to other developers."),
		mcp.WithString("pkg", mcp.Description("The package path to audit"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	docs, err := GenerateDocs(ctx, pkg)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%+v", docs)), nil
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

