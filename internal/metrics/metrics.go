package metrics

import (
	"context"
	"fmt"
	"go/ast"
	"mcp-server-go-refactor/internal/registry"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/tools/go/packages"
)

// Tool implements the complexity analyzer tool.
type Tool struct{}

// Register adds the complexity analyzer tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_complexity_analyzer",
		mcp.WithDescription("Calculates cyclomatic complexity for all functions in a package."),
		mcp.WithString("pkg", mcp.Description("The package path to analyze"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	result, err := CalculateComplexity(ctx, pkg)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
}

// MetricResult contains complexity scores for all functions in the package.
type MetricResult struct {
	TotalComplexity int              `json:"TotalComplexity"`
	Functions       map[string]int `json:"Functions"`
}

// CalculateComplexity runs cyclomatic complexity analysis on all functions.
func CalculateComplexity(ctx context.Context, pkgPath string) (*MetricResult, error) {
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

	funcs := make(map[string]int)
	total := 0

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				if f, ok := n.(*ast.FuncDecl); ok {
					c := calcComplexity(f)
					funcs[f.Name.Name] = c
					total += c
				}
				return true
			})
		}
	}

	return &MetricResult{
		TotalComplexity: total,
		Functions:       funcs,
	}, nil
}

func calcComplexity(f *ast.FuncDecl) int {
	c := 1
	if f.Body == nil {
		return c
	}
	ast.Inspect(f.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			c++
		case *ast.BinaryExpr:
			if x.Op.String() == "&&" || x.Op.String() == "||" {
				c++
			}
		}
		return true
	})
	return c
}
