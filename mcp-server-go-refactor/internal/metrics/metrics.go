package metrics

import (
	"context"
	"fmt"
	"go/ast"
	"sort"
	"strings"

	"mcp-server-go-refactor/internal/config"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"

	"github.com/fzipp/gocyclo"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/uudashr/gocognit"
)

// Tool implements the complexity analyzer tool.
type Tool struct{}

// Register adds the complexity analyzer tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_complexity_analyzer",
		mcp.WithDescription("Performs deep static analysis to calculate cyclomatic and cognitive complexity scores for every function in a package. Use this to identify \"God functions\" and deeply nested logic that hinders maintainability. It provides specific refactoring targets based on industry-standard complexity thresholds."),
		mcp.WithString("pkg", mcp.Description("The package path to analyze"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	result, err := CalculateComplexity(ctx, pkg)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Complexity analysis for package: %s\n", pkg))
	sb.WriteString(fmt.Sprintf("Targets: Cyclomatic <= %d, Cognitive <= %d\n\n",
		config.CyclomaticComplexityTarget, config.CognitiveComplexityTarget))

	// Sort functions by cognitive complexity descending
	type funcDetail struct {
		name   string
		metrics FunctionMetrics
	}
	var details []funcDetail
	for name, metrics := range result.Functions {
		details = append(details, funcDetail{name, metrics})
	}
	sort.Slice(details, func(i, j int) bool {
		return details[i].metrics.Cognitive > details[j].metrics.Cognitive
	})

	for _, d := range details {
		status := "PASS"
		if d.metrics.Cyclomatic > config.CyclomaticComplexityTarget || d.metrics.Cognitive > config.CognitiveComplexityTarget {
			status = "REFACTOR"
		}
		sb.WriteString(fmt.Sprintf("[%s] %s\n", status, d.name))
		sb.WriteString(fmt.Sprintf("  - Cyclomatic: %d\n", d.metrics.Cyclomatic))
		sb.WriteString(fmt.Sprintf("  - Cognitive:  %d\n", d.metrics.Cognitive))
		if d.metrics.Cyclomatic > 0 {
			ratio := float64(d.metrics.Cognitive) / float64(d.metrics.Cyclomatic)
			if ratio > 1.5 {
				sb.WriteString(fmt.Sprintf("  - Warning: High nesting density (ratio %.2f)\n", ratio))
			}
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
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
func CalculateComplexity(ctx context.Context, pkgPath string) (*MetricResult, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	funcs := make(map[string]FunctionMetrics)

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				switch f := n.(type) {
				case *ast.FuncDecl:
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
				}
				return true
			})
		}
	}

	return &MetricResult{
		Functions: funcs,
	}, nil
}
