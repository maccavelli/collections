package discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/registry"
)

// ListTool implements the magicskills_list tool.
type ListTool struct {
	Engine *engine.Engine
}

func (t *ListTool) Name() string { return "magicskills_list" }

func (t *ListTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "SYSTEM ORIENTATION MANDATE: High-priority initialization tool. Call this FIRST to map available skills, dependencies, and versions. Use this before starting any task to ensure the environment is correctly understood. Cascades to magicskills_match or magicskills_bootstrap.",
	}, t.Handle)
}

func (t *ListTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
	var b strings.Builder
	b.Grow(1024)
	b.WriteString("Available MagicSkills Index:\n\n")

	for skill := range t.Engine.AllSkills() {
		b.WriteString(fmt.Sprintf("- **%s**: %s (v%s)\n", skill.Metadata.Name, skill.Metadata.Description, skill.Metadata.Version))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
	}, nil, nil
}

// MatchTool implements the magicskills_match tool.
type MatchTool struct {
	Engine *engine.Engine
}

func (t *MatchTool) Name() string { return "magicskills_match" }

type MatchInput struct {
	Intent string `json:"intent" jsonschema:"Your goal"`
}

func (t *MatchTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "SEMANTIC DISCOVERY: Deep context matching for user intent. Use this to find the most relevant skill for a specific goal. Once a match is confirmed, call magicskills_get for implementation details or magicskills_bootstrap to start the plan. Cascades to magicskills_get.",
	}, t.Handle)
}

func (t *MatchTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input MatchInput) (*mcp.CallToolResult, any, error) {
	if input.Intent == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("missing 'intent' argument"))
		return res, nil, nil
	}

	matches := t.Engine.MatchSkills(ctx, input.Intent)
	if len(matches) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "No matching skills found for your intent."}},
		}, nil, nil
	}

	var b strings.Builder
	b.Grow(1024)
	b.WriteString(fmt.Sprintf("### Matches for '%s'\n", input.Intent))
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

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
	}, nil, nil
}

// Register registers discovery tools with the global registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&ListTool{Engine: eng})
	registry.Global.Register(&MatchTool{Engine: eng})
}

