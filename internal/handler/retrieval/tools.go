package retrieval

import (
	"context"
	"fmt"
	"strings"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/models"
	"mcp-server-magicskills/internal/registry"
	"mcp-server-magicskills/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/mod/semver"
)

// GetTool implements the magicskills_get tool.
type GetTool struct {
	Engine *engine.Engine
}

func (t *GetTool) Name() string { return "magicskills_get" }

// GetInput defines the structural representation for the entity.
type GetInput struct {
	util.UniversalBaseInput
	Name    string `json:"name" jsonschema:"The name of the skill to retrieve"`
	Section string `json:"section,omitempty" jsonschema:"Optional granular section to retrieve"`
	Version string `json:"version,omitempty" jsonschema:"Optional minimum semver bound (e.g. 1.2.0)"`
}

func (t *GetTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "Retrieve the full content of a skill by name, including parsed metadata, version-validated rules, and structured output. This is the canonical interface for loading skills — provides semver validation, section extraction, and error handling that raw file reads cannot. Use this instead of view_file on SKILL.md files.",
	}, t.Handle)
}

func (t *GetTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, any, error) {
	if err := t.Engine.WaitReady(ctx); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("engine initialization aborted: %w", err))
		return res, nil, nil
	}
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

	if input.Version != "" && skill.Metadata.Version != "" {
		vBound := input.Version
		vSkill := skill.Metadata.Version
		if !strings.HasPrefix(vBound, "v") {
			vBound = "v" + vBound
		}
		if !strings.HasPrefix(vSkill, "v") {
			vSkill = "v" + vSkill
		}
		if semver.IsValid(vBound) && semver.IsValid(vSkill) {
			if semver.Compare(vSkill, vBound) < 0 {
				res := &mcp.CallToolResult{}
				res.SetError(fmt.Errorf("skill version %s is older than requested bound %s", skill.Metadata.Version, input.Version))
				return res, nil, nil
			}
		}
	}

	output := struct {
		*models.Skill
	}{
		Skill: skill,
	}

	section := strings.ToLower(input.Section)
	summary := fmt.Sprintf("Retrieved skill: %s", input.Name)
	if section != "" {
		summary = fmt.Sprintf("Retrieved section '%s' for skill: %s", section, input.Name)
	}

	return &mcp.CallToolResult{}, struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	}{
		Summary: summary,
		Data:    output,
	}, nil
}

// Register registers retrieval tools with the global registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&GetTool{Engine: eng})
}
