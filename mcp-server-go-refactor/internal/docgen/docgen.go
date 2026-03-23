package docgen

import (
	"context"
	"fmt"
	"go/ast"
	"mcp-server-go-refactor/internal/registry"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/tools/go/packages"
)

// Tool implements the doc generator tool.
type Tool struct{}

// Register adds the doc generator tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_doc_generator",
		mcp.WithDescription("Audits missing documentation for exported identifiers."),
		mcp.WithString("pkg", mcp.Description("The package path to audit"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	_ = EnsureValidPkgPath(pkg)
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
	cfg := &packages.Config{
		Mode:    packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Tests:   true,
		Context: ctx,
	}
	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load package: %v", err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no package found at %s", pkgPath)
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

// EnsureValidPkgPath formats a package path for Go analysis.
func EnsureValidPkgPath(p string) string {
	if !strings.HasPrefix(p, "./") && !strings.Contains(p, ".") {
		return "./" + p
	}
	return p
}
