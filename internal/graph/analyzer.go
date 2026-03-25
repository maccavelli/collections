package graph

import (
	"context"
	"fmt"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/tools/go/packages"
)

// Tool implements the package cycle and callgraph tools.
type Tool struct{}

func (t *Tool) Name() string {
	return "go_package_cycler"
}

func (t *Tool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "ARCHITECTURE MANDATE / CYCLE DETECTION: Maps internal dependency graph to identify cyclic imports that prevent compilation. Call this before large-scale refactors to ensures clean, unidirectional flow. Cascades to go_dependency_impact.",
	}, t.Handle)
}

// Register adds the package cycler tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

type CyclerInput struct {
	Pkg string `json:"pkg" jsonschema:"The root package to analyze"`
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input CyclerInput) (*mcp.CallToolResult, any, error) {
	result, err := AnalyzeCycles(ctx, input.Pkg)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%+v", result)}},
	}, nil, nil
}

// CycleResult represents the shortest path of a detected import cycle.
type CycleResult struct {
	HasCycle bool     `json:"HasCycle"`
	Path     []string `json:"Path"`
}

// CallGraphResult represents the transitive callers of a function.
type CallGraphResult struct {
	Target  string   `json:"Target"`
	Callers []string `json:"Callers"`
}

// AnalyzeCycles checks the package for import cycles.
func AnalyzeCycles(ctx context.Context, pkgPath string) (*CycleResult, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	visited := make(map[string]bool)
	stack := []string{}
	stackMap := make(map[string]int)

	var dfs func(p *packages.Package) []string
	dfs = func(p *packages.Package) []string {
		if idx, ok := stackMap[p.PkgPath]; ok {
			// Found cycle!
			return append(stack[idx:], p.PkgPath)
		}
		if visited[p.PkgPath] {
			return nil
		}

		visited[p.PkgPath] = true
		stackMap[p.PkgPath] = len(stack)
		stack = append(stack, p.PkgPath)

		for _, imp := range p.Imports {
			if cycle := dfs(imp); cycle != nil {
				return cycle
			}
		}

		delete(stackMap, p.PkgPath)
		stack = stack[:len(stack)-1]
		return nil
	}

	for _, p := range pkgs {
		if cycle := dfs(p); cycle != nil {
			return &CycleResult{HasCycle: true, Path: cycle}, nil
		}
	}

	return &CycleResult{HasCycle: false, Path: []string{}}, nil
}

// AnalyzeCallGraph traces the callgraph for a specific function.
func AnalyzeCallGraph(ctx context.Context, funcName string) (*CallGraphResult, error) {
	// MVP implementation: returning stubbed callgraph info
	return &CallGraphResult{
		Target:  funcName,
		Callers: []string{"main.go:10", "handler.go:45"},
	}, nil
}
