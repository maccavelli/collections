package dependency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool implements the dependency impact tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_dependency_impact"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] DEPENDENCY IMPACT ANALYZER: Evaluates risk of external packages. Analyzes go.mod files for outdated dependencies by mapping transitive influence and vulnerability risks. Produces module dependency list with update availability. [Routing Tags: dependencies, modules, go.mod, impact-analysis, check-updates]",
	}, t.Handle)
}

// Register adds the dependency impact tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

type ImpactInput struct {
	models.UniversalPipelineInput
}

func (t *Tool) Handle(ctx context.Context, _ *mcp.CallToolRequest, input ImpactInput) (*mcp.CallToolResult, any, error) {
	pkg := input.Target
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, pkg)

		if recallAvailable {
			if history := t.Engine.LoadCrossSessionFromRecall(ctx, "gorefactor", pkg); history != "" {
				if session.Metadata == nil {
					session.Metadata = make(map[string]any)
				}
				session.Metadata["historical_dependencies"] = history
			}
		}
	}

	impact, err := Analyze(ctx, pkg)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	summary := fmt.Sprintf("Dependency analysis for %s found %d modules", pkg, len(impact.Modules))

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			depStds := t.Engine.EnsureRecallCache(ctx, session, "dependency_mgmt", "search", map[string]interface{}{"namespace": "ecosystem",
				"query": "Go module dependency management standards, version pinning policies, and upgrade impact analysis rules for " + pkg,
				"limit": 15,
			})
			session.Metadata["recall_cache_dependency"] = depStds

			if depStds != "" {
				summary += fmt.Sprintf("\n\n[Dependency Management Standards]: %s", depStds)
			}
		}

		// Pillar metrics for brainstorm learning.
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":       "modularization",
			"module_count": len(impact.Modules),
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, pkg, "dependencies_analyzed", "native", "go_dependency_impact", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string  `json:"summary"`
		Data    *Impact `json:"data"`
	}{
		Summary: summary,
		Data:    impact,
	}, nil
}

// Module represents the JSON output from go list.
type Module struct {
	Path    string  `json:"Path"`
	Version string  `json:"Version"`
	Time    string  `json:"Time"`
	Update  *Module `json:"Update"`
}

// Impact represents the result of the dependency analysis.
type Impact struct {
	TargetModule string
	Modules      []Module
}

// Analyze runs the dependency impact check.
func Analyze(ctx context.Context, pkg string) (*Impact, error) {
	res, err := loader.Discover(ctx, pkg)
	if err != nil {
		return nil, err
	}

	// For dependency analysis, a "." pattern resolved by Discover means the current module
	// We want 'go list -m' with the resolved pattern.
	p := res.Pattern
	if p == "." {
		p = "all" // In terminal, 'go list -m all' is the safest way to get all including current
	}
	out, err := res.Runner.RunGo(ctx, "list", "-m", "-u", "-json", p)
	if err != nil {
		return nil, err
	}

	var mods []Module
	if len(out.Stdout) > 0 {
		dec := json.NewDecoder(bytes.NewReader(out.Stdout))
		for dec.More() {
			var mod Module
			if err := dec.Decode(&mod); err == nil {
				mods = append(mods, mod)
			}
		}
	}

	if len(mods) == 0 {
		// Fallback: if 'go list -m all' didn't work as expected, try just the module name
		out, err = res.Runner.RunGo(ctx, "list", "-m", "-json", res.Workspace.ModuleName)
		if err == nil {
			var mod Module
			if err := json.Unmarshal(out.Stdout, &mod); err == nil {
				mods = append(mods, mod)
			}
		}
	}

	if len(mods) == 0 {
		return nil, fmt.Errorf("no module info found for %s (runner dir: %s, pattern: %s)", pkg, res.Runner.Dir, p)
	}

	return &Impact{
		TargetModule: pkg,
		Modules:      mods,
	}, nil
}
