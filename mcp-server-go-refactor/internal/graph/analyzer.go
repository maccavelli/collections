package graph

import (
	"context"
	"fmt"
	"log/slog"
	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/tools/go/packages"
)

// Tool implements the package cycle and callgraph tools.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_package_cycler"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] CYCLIC IMPORT DETECTOR: Traces, finds, and reports import cycles by walking the package dependency graph. Detects circular dependencies that cause compilation failures and suggest restructuring. Produces cycle detection result for dependency restructuring.",
	}, t.Handle)
}

// Register adds the package cycler tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

type CyclerInput struct {
	models.UniversalPipelineInput
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input CyclerInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Target)
	}

	result, err := AnalyzeCycles(ctx, input.Target)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	summary := "No import cycles detected."
	if result.HasCycle {
		summary = fmt.Sprintf("Import cycle detected in %s: %v", input.Target, result.Path)
	}

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			cycleStds := t.Engine.EnsureRecallCache(ctx, session, "import_cycles", "search", map[string]interface{}{"namespace": "ecosystem",
				"query": "Go import cycle resolution standards, dependency inversion patterns, and package structure conventions for " + input.Target,
				"limit": 10,
			})
			session.Metadata["recall_cache_cycles"] = cycleStds

			if cycleStds != "" {
				summary += fmt.Sprintf("\n\n[Import Cycle Resolution Standards]: %s", cycleStds)
			}
		}

		// Pillar metrics for brainstorm learning.
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":            "modularization",
			"has_cycles":        result.HasCycle,
			"cycle_path_length": len(result.Path),
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)

		// AST faults.
		if result.HasCycle {
			var astFaults []string
			if f, ok := session.Metadata["ast_faults"].([]string); ok {
				astFaults = f
			}
			session.Metadata["ast_faults"] = append(astFaults, "cyclic_import")
		}

		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "cycles_detected", "native", "go_package_cycler", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string       `json:"summary"`
		Data    *CycleResult `json:"data"`
	}{
		Summary: summary,
		Data:    result,
	}, nil
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
