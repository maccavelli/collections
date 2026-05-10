package discovery

import (
	"context"
	"fmt"
	"mcp-server-magicskills/internal/util"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/registry"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListTool implements the magicskills_list tool.
type ListTool struct {
	Engine *engine.Engine
}

func (t *ListTool) Name() string { return "magicskills_list" }

func (t *ListTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "List all available skills with their names, descriptions, and versions. Skills are reusable markdown instruction files that extend agent capabilities for specialized tasks. This is the canonical skill catalog — use this instead of scanning the filesystem for SKILL.md files.",
	}, t.Handle)
}

func (t *ListTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
	if err := t.Engine.WaitReady(ctx); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("engine initialization aborted: %w", err))
		return res, nil, nil
	}
	var skills []any
	for skill := range t.Engine.AllSkills() {
		skills = append(skills, map[string]string{
			"name":        skill.Metadata.Name,
			"description": skill.Metadata.Description,
			"version":     skill.Metadata.Version,
		})
	}

	summary := fmt.Sprintf("Found %d available MagicSkills", len(skills))
	return &mcp.CallToolResult{}, struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	}{
		Summary: summary,
		Data: map[string]any{
			"skills": skills,
		},
	}, nil
}

// MatchTool implements the magicskills_match tool.
type MatchTool struct {
	Engine *engine.Engine
}

func (t *MatchTool) Name() string { return "magicskills_match" }

// MatchInput defines the structural representation for the entity.
type MatchInput struct {
	util.UniversalBaseInput
	Intent   string `json:"intent" jsonschema:"Your goal"`
	Category string `json:"category,omitempty" jsonschema:"Optional category or domain filter (e.g. 'go', 'python')"`
	Target   string `json:"target,omitempty" jsonschema:"Optional target workspace root to dynamically constrain index bounds"`
}

func (t *MatchTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "Search for skills matching a goal or intent using semantic matching. Returns the top matching skills ranked by relevance score. Use this instead of manually scanning skill names from the system prompt. Follow up with magicskills_get to retrieve the full instructions.",
	}, t.Handle)
}

func (t *MatchTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input MatchInput) (*mcp.CallToolResult, any, error) {
	if err := t.Engine.WaitReady(ctx); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("engine initialization aborted: %w", err))
		return res, nil, nil
	}
	if input.Intent == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("missing 'intent' argument"))
		return res, nil, nil
	}

	matches := t.Engine.MatchSkills(ctx, input.Intent, input.Category, input.Target, 3)

	summary := "No matching skills found for your intent."
	if len(matches) > 0 {
		summary = fmt.Sprintf("Found %d matching skills for intent: %s", len(matches), input.Intent)
	}

	var matchData []map[string]any
	for _, m := range matches {
		matchData = append(matchData, map[string]any{
			"name":        m.Skill.Metadata.Name,
			"description": m.Skill.Metadata.Description,
			"score":       m.Score,
			"tags":        m.Skill.Metadata.Tags,
		})
	}

	return &mcp.CallToolResult{}, struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	}{
		Summary: summary,
		Data: map[string]any{
			"intent":  input.Intent,
			"matches": matchData,
		},
	}, nil
}

// Register registers discovery tools with the global registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&ListTool{Engine: eng})
	registry.Global.Register(&MatchTool{Engine: eng})
}
