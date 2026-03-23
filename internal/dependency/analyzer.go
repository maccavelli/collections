package dependency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mcp-server-go-refactor/internal/registry"
	"os/exec"
	"time"

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
		mcp.WithDescription("Analyzes the blast radius of a module version update using 'go list -m -u'."),
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
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(tctx, "go", "list", "-m", "-u", "-json", pkg)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil && out.Len() == 0 {
		return nil, err
	}

	var mod Module
	if err := json.Unmarshal(out.Bytes(), &mod); err != nil {
		return nil, err
	}

	return &Impact{
		TargetModule: pkg,
		Modules:      []Module{mod},
	}, nil
}
