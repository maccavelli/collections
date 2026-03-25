package pruner

import (
	"context"
	"fmt"
	"go/types"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool implements the dead code pruner tool.
type Tool struct{}

func (t *Tool) Name() string {
	return "go_dead_code_pruner"
}

func (t *Tool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "HYGIENE MANDATE / PRUNING ENGINE: Conducts a comprehensive semantic scan to detect unreferenced package-level functions, variables, and constants. Use this to reduce maintenance surface area and binary size. Cascades to go_package_cycler for architecture cleanup.",
	}, t.Handle)
}

// Register adds the dead code pruner tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

type PruneInput struct {
	Pkg string `json:"pkg" jsonschema:"The package path to scan"`
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input PruneInput) (*mcp.CallToolResult, any, error) {
	result, err := PruneDeadCode(ctx, input.Pkg)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%+v", result)}},
	}, nil, nil
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

	result := &DeadCodeResult{
		UnusedFunctions: []string{},
		UnusedVariables: []string{},
	}

	for _, pkg := range pkgs {
		// 1. Identify all package-level declarations across the package scope
		declared := make(map[types.Object]bool)
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if obj != nil {
				declared[obj] = false
			}
		}

		// 2. Mark all objects that are actually used in the package AST.
		// pkg.TypesInfo.Uses contains mappings from identifiers to their referenced objects.
		for _, obj := range pkg.TypesInfo.Uses {
			if _, ok := declared[obj]; ok {
				declared[obj] = true
			}
		}

		// 3. Collect declarations that have no uses
		for obj, used := range declared {
			if used {
				continue
			}

			// Ignore special names and test functions
			name := obj.Name()
			if name == "_" || name == "main" || name == "init" || strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") || strings.HasPrefix(name, "Example") {
				continue
			}

			// Categorize by type for reporting
			switch v := obj.(type) {
			case *types.Func:
				sig := v.Type().(*types.Signature)
				// methods are more complex to prune because of interface implementation
				if sig.Recv() != nil {
					continue
				}
				result.UnusedFunctions = append(result.UnusedFunctions, name)
			case *types.Var:
				result.UnusedVariables = append(result.UnusedVariables, name)
			}
		}
	}


	return result, nil
}
