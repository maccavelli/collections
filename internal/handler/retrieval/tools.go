package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/mod/semver"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/models"
	"mcp-server-magicskills/internal/registry"
)

// GetTool implements the magicskills_get tool.
type GetTool struct {
	Engine *engine.Engine
}

func (t *GetTool) Name() string { return "magicskills_get" }

type GetInput struct {
	Name    string `json:"name" jsonschema:"The name of the skill to retrieve"`
	Section string `json:"section" jsonschema:"Optional granular section to retrieve"`
	Version string `json:"version" jsonschema:"Optional minimum semver bound (e.g. 1.2.0)"`
}

func (t *GetTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "KNOWLEDGE DEEP-DIVE: Primary retrieval for skill-defined logic and workflows. Call this after magicskills_match to extract granular architectural details. Essential for planning. Cascades to magicskills_bootstrap.",
	}, t.Handle)
}

func (t *GetTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, any, error) {
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

	section := strings.ToLower(input.Section)
	if section != "" {
		content, found := skill.Sections[section]
		if !found {
			for k, v := range skill.Sections {
				if strings.Contains(k, section) {
					content = v
					found = true
					break
				}
			}
		}
		if found {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("### %s: %s\n\n%s", input.Name, section, engine.Densify(content))}},
			}, nil, nil
		}
	}

	return newHybridResult(skill), nil, nil
}

func newHybridResult(skill *models.Skill) *mcp.CallToolResult {
	type meta struct {
		Name          string    `json:"name"`
		Version       string    `json:"version"`
		SchemaVersion string    `json:"schema_version"`
		Hash          string    `json:"hash"`
		TokenEstimate int       `json:"token_estimate"`
		UpdatedAt     time.Time `json:"updated_at"`
	}

	m := meta{
		Name:          skill.Metadata.Name,
		Version:       skill.Metadata.Version,
		SchemaVersion: skill.SchemaVersion,
		Hash:          skill.Hash,
		TokenEstimate: skill.TokenEstimate,
		UpdatedAt:     skill.UpdatedAt,
	}

	metaJSON, err := json.Marshal(m)
	if err != nil {
		metaJSON = []byte(fmt.Sprintf(`{"error": "failed to serialize metadata: %v"}`, err))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(metaJSON)},
			&mcp.TextContent{Text: skill.Digest},
		},
	}
}

// Register registers retrieval tools with the global registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&GetTool{Engine: eng})
}

