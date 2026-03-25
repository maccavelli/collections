package contextanalysis

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tool implements the context propagation analyzer tool.
type Tool struct{}

// Register adds the context propagation analyzer tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_context_analyzer",
		mcp.WithDescription("Audits call chains to ensure robust context propagation, identifying where parent contexts are dropped in favor of context.Background() or context.TODO(). This is critical for ensuring proper cancellation, timeouts, and tracing across distributed systems or concurrent operations."),
		mcp.WithString("pkg", mcp.Description("The package path"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	findings, err := AnalyzeContext(ctx, pkg)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if len(findings) == 0 {
		return mcp.NewToolResultText("No context propagation issues found."), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%+v", findings)), nil
}

type Finding struct {
	File      string `json:"File"`
	Line      int    `json:"Line"`
	Function  string `json:"Function"`
	Rationale string `json:"Rationale"`
}

func AnalyzeContext(ctx context.Context, pkgPath string) ([]Finding, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	findings := []Finding{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				// Check if calling a function that takes context as first param
				// but passing context.Background() or context.TODO()
				if isContextDeprivedCall(pkg.TypesInfo, call) {
					pos := pkg.Fset.Position(call.Pos())
					findings = append(findings, Finding{
						File:      pos.Filename,
						Line:      pos.Line,
						Function:  fmt.Sprintf("%v", call.Fun),
						Rationale: "Call ignores potential parent context by using context.Background() or context.TODO().",
					})
				}
				return true
			})
		}
	}
	return findings, nil
}

func isContextDeprivedCall(info *types.Info, call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}

	// 1. Check if the first argument is specifically context.Background() or context.TODO()
	firstArg := call.Args[0]
	argCall, ok := firstArg.(*ast.CallExpr)
	if !ok {
		return false
	}

	sel, ok := argCall.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	if sel.Sel.Name != "Background" && sel.Sel.Name != "TODO" {
		return false
	}

	// Verify it's the "context" package
	if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "context" {
		return false
	}

	// 2. STRENGTHEN: Check if the function being called actually expects context.Context as first argument.
	// This uses type info to avoid false positives.
	var funObj types.Object
	if info != nil {
		switch f := call.Fun.(type) {
		case *ast.Ident:
			funObj = info.Uses[f]
		case *ast.SelectorExpr:
			funObj = info.Uses[f.Sel]
		}
	}

	if funObj == nil {
		return true // Fallback to naive if type info missing
	}

	sig, ok := funObj.Type().Underlying().(*types.Signature)
	if !ok || sig.Params().Len() == 0 {
		return false
	}

	firstParam := sig.Params().At(0)
	paramType := firstParam.Type().String()
	
	// Check if param type is context.Context
	return strings.Contains(paramType, "context.Context")
}

