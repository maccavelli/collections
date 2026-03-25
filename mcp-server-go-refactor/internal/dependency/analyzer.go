package dependency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tool implements the dependency impact tool.
type Tool struct{}

// Register adds the dependency impact tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_dependency_impact",
		mcp.WithDescription("Evaluates the \"blast radius\" of updating external dependencies by checking for available updates and mapping their transitive influence. Use this before upgrading core libraries to understand which parts of your system might require regression testing."),
		mcp.WithString("pkg", mcp.Description("The package path to analyze"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	if pkg == "" {
		return nil, fmt.Errorf("argument 'pkg' is required")
	}
	impact, err := Analyze(ctx, pkg)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%+v", impact)), nil
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
