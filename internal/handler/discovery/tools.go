package discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/registry"
)

// ListTool implements the magicskills_list tool.
type ListTool struct {
	Engine *engine.Engine
}

func (t *ListTool) Metadata() mcp.Tool {
	return mcp.NewTool("magicskills_list",
		mcp.WithDescription("Lists all available skills in the authoritative index."),
	)
}

func (t *ListTool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var b strings.Builder
	b.Grow(1024)
	b.WriteString("Available MagicSkills Index:\n\n")

	for skill := range t.Engine.AllSkills() {
		b.WriteString(fmt.Sprintf("- **%s**: %s (v%s)\n", skill.Metadata.Name, skill.Metadata.Description, skill.Metadata.Version))
	}

	return mcp.NewToolResultText(b.String()), nil
}

// MatchTool implements the magicskills_match tool.
type MatchTool struct {
	Engine *engine.Engine
}

func (t *MatchTool) Metadata() mcp.Tool {
	return mcp.NewTool("magicskills_match",
		mcp.WithDescription("Finds matching skills based on your goal and returns a dense digest immediately."),
		mcp.WithString("intent", mcp.Description("Your goal"), mcp.Required()),
	)
}

func (t *MatchTool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	intent := request.GetString("intent", "")
	if intent == "" {
		return mcp.NewToolResultError("missing 'intent' argument"), nil
	}

	matches := t.Engine.MatchSkills(ctx, intent)
	if len(matches) == 0 {
		return mcp.NewToolResultText("No matching skills found for your intent."), nil
	}

	var b strings.Builder
	b.Grow(1024)
	b.WriteString(fmt.Sprintf("### Matches for '%s'\n", intent))
	for i, m := range matches {
		indicator := ""
		if i == 0 {
			indicator = " (Direct match recommended)"
		}
		b.WriteString(fmt.Sprintf("- **%s**: %s%s\n", m.Metadata.Name, m.Metadata.Description, indicator))
	}

	b.WriteString("\n---\n")
	b.WriteString("### Best Match Digest\n")
	b.WriteString(matches[0].Digest)

	return mcp.NewToolResultText(b.String()), nil
}

// Register registers discovery tools with the global registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&ListTool{Engine: eng})
	registry.Global.Register(&MatchTool{Engine: eng})
}
