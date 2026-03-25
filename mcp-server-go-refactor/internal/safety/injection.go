package safety

import (
	"context"
	"fmt"
	"go/ast"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool implements the SQL injection guard tool.
type Tool struct{}

func (t *Tool) Name() string {
	return "go_sql_injection_guard"
}

func (t *Tool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "SECURITY MANDATE / INJECTION GUARD: Performs safety-focused AST analysis to detect unsafe dynamic SQL string construction. Use this to identify potential SQL injection vectors where user input might be concatenated into queries. Essential for security audits of API and DB layers.",
	}, t.Handle)
}

// Register adds the injection guard tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

type InjectionInput struct {
	Pkg string `json:"pkg" jsonschema:"The package path to scan"`
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input InjectionInput) (*mcp.CallToolResult, any, error) {
	result, err := DetectInjections(ctx, input.Pkg)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%+v", result)}},
	}, nil, nil
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
