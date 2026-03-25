package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/registry"
)

// BootstrapTool implements the magicskills_bootstrap tool.
type BootstrapTool struct {
	Engine *engine.Engine
}

func (t *BootstrapTool) Metadata() mcp.Tool {
	return mcp.NewTool("magicskills_bootstrap",
		mcp.WithDescription("Synthesizes an actionable markdown task checklist from a skill's defined workflow or requirements. This is a critical bridge between documentation and execution, allowing an agent to track progress through a multi-step procedure. Use this at the start of any complex task to ensure all required steps are identified and tracked."),
		mcp.WithString("name", mcp.Description("The skill name"), mcp.Required()),
	)
}

func (t *BootstrapTool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("missing 'name' argument"), nil
	}

	skill, ok := t.Engine.GetSkill(name)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("skill not found: %s", name)), nil
	}

	content, ok := skill.Sections["workflow"]
	if !ok {
		content, ok = skill.Sections["checklist"]
	}
	if !ok {
		return mcp.NewToolResultText("No workflow or checklist found in skill."), nil
	}

	var checklist strings.Builder
	checklist.Grow(len(content))
	lines := strings.Split(content, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			checklist.WriteString("- [ ] " + strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* ") + "\n")
		} else if len(trimmed) > 2 && trimmed[1] == '.' {
			checklist.WriteString("- [ ] " + trimmed[3:] + "\n")
		}
	}

	if checklist.Len() == 0 {
		return mcp.NewToolResultText("Found workflow section but no bullet points to bootstrap."), nil
	}

	return mcp.NewToolResultText("# Tasks\n\n" + checklist.String()), nil
}

// Register registers bootstrap tools with the global registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&BootstrapTool{Engine: eng})
}
