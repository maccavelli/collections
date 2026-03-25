package system

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/registry"
	"mcp-server-magicskills/internal/scanner"
)

// ValidateDepsTool implements the magicskills_validate_deps tool.
type ValidateDepsTool struct {
	Engine *engine.Engine
}

func (t *ValidateDepsTool) Name() string { return "magicskills_validate_deps" }

type ValidateDepsInput struct {
	Name string `json:"name" jsonschema:"The skill name"`
}

func (t *ValidateDepsTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "PRE-EXECUTION AUDIT: Requisite check to verify host environment readiness before executing any skill-defined commands. Must be called after magicskills_bootstrap. Cascades to implementation tools.",
	}, t.Handle)
}

func (t *ValidateDepsTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input ValidateDepsInput) (*mcp.CallToolResult, any, error) {
	if input.Name == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("missing 'name' argument"))
		return res, nil, nil
	}

	skill, ok := t.Engine.GetSkill(input.Name)
	if !ok {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("skill not found: %s", input.Name))
		return res, nil, nil
	}

	if len(skill.Metadata.Requirements) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Skill '%s' has no specific host binary requirements.", input.Name)}},
		}, nil, nil
	}

	var missing []string
	var found []string
	for _, req := range skill.Metadata.Requirements {
		if _, err := exec.LookPath(req); err != nil {
			missing = append(missing, req)
		} else {
			found = append(found, req)
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Dependencies Check for '%s':\n", input.Name))
	if len(missing) > 0 {
		b.WriteString("\n**MISSING BINARIES**:\n")
		for _, m := range missing {
			b.WriteString(fmt.Sprintf("- %s\n", m))
		}
	}
	if len(found) > 0 {
		b.WriteString("\n**FOUND BINARIES**:\n")
		for _, f := range found {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}

	if len(missing) > 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
		}, nil, nil
	}
	b.WriteString("\nAll dependencies met. Ready to execute.")
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
	}, nil, nil
}

// AddRootTool implements the magicskills_add_root tool.
type AddRootTool struct {
	Scanner *scanner.Scanner
	Engine  *engine.Engine
}

func (t *AddRootTool) Name() string { return "magicskills_add_root" }

type AddRootInput struct {
	Path string `json:"path" jsonschema:"Absolute path to index"`
}

func (t *AddRootTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "WORKSPACE EXPANSION: Dynamically expands indexing reach. Use this if required context or skill folders are missing from the current inventory. Initiates immediate re-scanning. Cascades to magicskills_list.",
	}, t.Handle)
}

func (t *AddRootTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input AddRootInput) (*mcp.CallToolResult, any, error) {
	if info, err := os.Stat(input.Path); err == nil && info.IsDir() {
		t.Scanner.Roots = append(t.Scanner.Roots, input.Path)
		files, err := t.Scanner.Discover(ctx)
		if err != nil {
			slog.Error("discovery error while adding manual root", "error", err)
			res := &mcp.CallToolResult{}
			res.SetError(fmt.Errorf("discovery error: %v", err))
			return res, nil, nil
		}
		if err := t.Engine.Ingest(ctx, files); err != nil {
			slog.Error("ingestion error while adding manual root", "error", err)
			res := &mcp.CallToolResult{}
			res.SetError(fmt.Errorf("ingestion error: %v", err))
			return res, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Added and indexed: %s", input.Path)}},
		}, nil, nil
	}
	res := &mcp.CallToolResult{}
	res.SetError(fmt.Errorf("invalid path: %s", input.Path))
	return res, nil, nil
}

// Register registers system tools with the global registry.
func Register(eng *engine.Engine, scn *scanner.Scanner) {
	registry.Global.Register(&ValidateDepsTool{Engine: eng})
	registry.Global.Register(&AddRootTool{Scanner: scn, Engine: eng})
}

