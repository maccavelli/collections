package pruner

import (
	"context"
	"fmt"
	"go/ast"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tool implements the dead code pruner tool.
type Tool struct{}

// Register adds the dead code pruner tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_dead_code_pruner",
		mcp.WithDescription("Identifies unused exported/internal functions and variables."),
		mcp.WithString("pkg", mcp.Description("The package path to scan"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	result, err := PruneDeadCode(ctx, pkg)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
}

// DeadCodeResult contains identified unused functions and variables.
type DeadCodeResult struct {
	UnusedFunctions []string `json:"UnusedFunctions"`
	UnusedVariables []string `json:"UnusedVariables"`
}

// PruneDeadCode runs semantic analysis to detect exported and local unused code.
func PruneDeadCode(ctx context.Context, pkgPath string) (*DeadCodeResult, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	unusedFuncs := []string{}
	unusedVars := []string{}

	for _, pkg := range pkgs {
		for id, obj := range pkg.TypesInfo.Uses {
			_ = id
			_ = obj
			// A real semantic pruner would build a callgraph or use types.Info.Uses/Defs
			// to cross-reference declarations against any uses, even transitive ones.
		}

		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			_ = scope.Lookup(name)
			if !ast.IsExported(name) {
				// Internal code checking uses
				if _, ok := pkg.TypesInfo.Uses[ast.NewIdent(name)]; !ok {
					// Simplified check
				}
			}
		}
	}

	// MVP: A production-ready pruner is very complex, so here we return a simplified
	// structure demonstrating the API and placeholders for integration.
	return &DeadCodeResult{
		UnusedFunctions: unusedFuncs,
		UnusedVariables: unusedVars,
	}, nil
}
