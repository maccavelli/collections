package safety

import (
	"context"
	"fmt"
	"go/ast"
	"mcp-server-go-refactor/internal/registry"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/tools/go/packages"
)

// Tool implements the SQL injection guard tool.
type Tool struct{}

// Register adds the injection guard tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_sql_injection_guard",
		mcp.WithDescription("Detects dynamic SQL string construction vulnerabilities."),
		mcp.WithString("pkg", mcp.Description("The package path to scan"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	result, err := DetectInjections(ctx, pkg)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
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
}

// DetectInjections analyzes SQL strings for dynamic concatenations.
func DetectInjections(ctx context.Context, pkgPath string) (*SQLInjectionResult, error) {
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
