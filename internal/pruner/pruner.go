// Package pruner provides functionality for the pruner subsystem.
package pruner

import (
	"context"
	"fmt"
	"go/types"
	"log/slog"
	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool implements the dead code pruner tool.
type Tool struct {
	Engine *engine.Engine
}

// Name performs the Name operation.
func (t *Tool) Name() string {
	return "go_dead_code_pruner"
}

// Register performs the Register operation.
func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] ORPHAN SYMBOL DETECTOR: Locates unreferenced exports. Eliminates, cleans, removes, and deletes unused symbols to reduce noise before downstream pipeline stages. WARNING: Deterministic AST mapping inherently generates false positives against reflection frameworks. Produces unused symbol list for cleanup planning. [REQUIRES: Completion of test coverage mappings] [TRIGGERS: brainstorm:peer_review] [Routing Tags: dead-code, orphan, unused-exports, prune, remove-code]",
	}, t.Handle)
}

// Register adds the dead code pruner tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

// PruneInput defines the PruneInput structure.
type PruneInput struct {
	models.UniversalPipelineInput
}

// Handle performs the Handle operation.
func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input PruneInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Target)

		if recallAvailable {
			if history := t.Engine.LoadCrossSessionFromRecall(ctx, "gorefactor", input.Target); history != "" {
				if session.Metadata == nil {
					session.Metadata = make(map[string]any)
				}
				session.Metadata["historical_dead_code"] = history
			}
		}
	}

	result, err := PruneDeadCode(ctx, input.Target)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	summary := fmt.Sprintf("Dead code analysis for %s found %d unused functions and %d unused variables", input.Target, len(result.UnusedFunctions), len(result.UnusedVariables))

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			prunerStds := t.Engine.EnsureRecallCache(ctx, session, "pruner_standards", "search", map[string]any{"namespace": "ecosystem",
				"query": "Go dead code removal conventions, API deprecation policies, export hygiene standards, and unused symbol management for " + input.Target,
				"limit": 15,
			})
			session.Metadata["recall_cache_pruner"] = prunerStds

			if prunerStds != "" {
				summary += fmt.Sprintf("\n\n[Dead Code & Deprecation Standards]: %s", prunerStds)
			}
		}

		// Pillar metrics for brainstorm learning.
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":           "efficiency",
			"unused_functions": len(result.UnusedFunctions),
			"unused_variables": len(result.UnusedVariables),
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)

		// AST faults.
		if len(result.UnusedFunctions) > 0 || len(result.UnusedVariables) > 0 {
			var astFaults []string
			if f, ok := session.Metadata["ast_faults"].([]string); ok {
				astFaults = f
			}
			session.Metadata["ast_faults"] = append(astFaults, "dead_code")
		}

		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "dead_code_scanned", "native", "go_dead_code_pruner", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string          `json:"summary"`
		Data    *DeadCodeResult `json:"data"`
	}{
		Summary: summary,
		Data:    result,
	}, nil
}

// DeadCodeResult contains identified unused functions and variables.
type DeadCodeResult struct {
	UnusedFunctions []string `json:"UnusedFunctions"`
	UnusedVariables []string `json:"UnusedVariables"`
}

// PruneDeadCode performs the PruneDeadCode operation.
func PruneDeadCode(ctx context.Context, pkgPath string) (*DeadCodeResult, error) {
	res, err := loader.Discover(ctx, pkgPath)
	if err != nil {
		return nil, err
	}

	// Load the holistic parent module root to establish correct global AST constraints
	// instead of strictly limiting the graph to the localized package.
	pkgs, err := loader.LoadPackagesWithResult(ctx, res, loader.DefaultMode, "./...")
	if err != nil {
		return nil, err
	}

	result := &DeadCodeResult{
		UnusedFunctions: []string{},
		UnusedVariables: []string{},
	}

	// 1. Enumerate all definitions synchronously across the holistic graph natively
	globalDeclared := make(map[types.Object]bool)
	for _, pkg := range pkgs {
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			if obj := scope.Lookup(name); obj != nil {
				globalDeclared[obj] = false
			}
		}
	}

	// 2. Iterate sequentially over AST boundaries mapping real structural 'Uses' traces
	for _, pkg := range pkgs {
		for _, obj := range pkg.TypesInfo.Uses {
			if _, ok := globalDeclared[obj]; ok {
				globalDeclared[obj] = true
			}
		}
	}

	// 3. Condense mapping payload and filter responses strictly to the caller's target
	cleanPath := strings.TrimPrefix(pkgPath, "./")
	for obj, used := range globalDeclared {
		if used {
			continue
		}

		if obj.Pkg() == nil || (!strings.HasSuffix(pkgPath, "...") && pkgPath != "." && pkgPath != cleanPath && !strings.Contains(obj.Pkg().Path(), cleanPath)) {
			// Skip flagged unused components belonging to dependency packages not explicitly queried
			continue
		}

		name := obj.Name()
		if name == "_" || name == "main" || name == "init" || strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") || strings.HasPrefix(name, "Example") {
			continue
		}

		switch v := obj.(type) {
		case *types.Func:
			sig := v.Type().(*types.Signature)
			if sig.Recv() != nil {
				continue
			}
			result.UnusedFunctions = append(result.UnusedFunctions, name)
		case *types.Var:
			result.UnusedVariables = append(result.UnusedVariables, name)
		}
	}

	return result, nil
}
