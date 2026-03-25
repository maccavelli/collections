package system

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/registry"
	"mcp-server-magicskills/internal/scanner"
)

// ValidateDepsTool implements the magicskills_validate_deps tool.
type ValidateDepsTool struct {
	Engine *engine.Engine
}

func (t *ValidateDepsTool) Metadata() mcp.Tool {
	return mcp.NewTool("magicskills_validate_deps",
		mcp.WithDescription("Audits the local host environment to verify the presence of specialized binary dependencies required by a skill (e.g., specific linters, runtimes, or CLI tools). Use this before attempting to execute a skill's workflow to prevent runtime failures and ensure the host environment is correctly provisioned."),
		mcp.WithString("name", mcp.Description("The skill name"), mcp.Required()),
	)
}

func (t *ValidateDepsTool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("missing 'name' argument"), nil
	}

	skill, ok := t.Engine.GetSkill(name)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("skill not found: %s", name)), nil
	}

	if len(skill.Metadata.Requirements) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("Skill '%s' has no specific host binary requirements.", name)), nil
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
	b.WriteString(fmt.Sprintf("Dependencies Check for '%s':\n", name))
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
		return mcp.NewToolResultText(b.String()), nil
	}
	b.WriteString("\nAll dependencies met. Ready to execute.")
	return mcp.NewToolResultText(b.String()), nil
}

// AddRootTool implements the magicskills_add_root tool.
type AddRootTool struct {
	Scanner *scanner.Scanner
	Engine  *engine.Engine
}

func (t *AddRootTool) Metadata() mcp.Tool {
	return mcp.NewTool("magicskills_add_root",
		mcp.WithDescription("Dynamically expands the MagicSkills knowledge base by adding a new directory root for discovery and indexing. Use this when new capability-folders are added to a workspace, ensuring that any contained workflows are immediately made available for discovery and matching."),
		mcp.WithString("path", mcp.Description("Absolute path to index"), mcp.Required()),
	)
}

func (t *AddRootTool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := request.GetString("path", "")
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		t.Scanner.Roots = append(t.Scanner.Roots, path)
		// We'll update the scanner to accept context in the next step
		files, err := t.Scanner.Discover(ctx)
		if err != nil {
			slog.Error("discovery error while adding manual root", "error", err)
			return mcp.NewToolResultError(fmt.Sprintf("discovery error: %v", err)), nil
		}
		if err := t.Engine.Ingest(ctx, files); err != nil {
			slog.Error("ingestion error while adding manual root", "error", err)
			return mcp.NewToolResultError(fmt.Sprintf("ingestion error: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Added and indexed: %s", path)), nil
	}
	return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %s", path)), nil
}

// Register registers system tools with the global registry.
func Register(eng *engine.Engine, scn *scanner.Scanner) {
	registry.Global.Register(&ValidateDepsTool{Engine: eng})
	registry.Global.Register(&AddRootTool{Scanner: scn, Engine: eng})
}
