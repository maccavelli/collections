package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/mod/semver"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/models"
	"mcp-server-magicskills/internal/registry"
)

// GetTool implements the magicskills_get tool.
type GetTool struct {
	Engine *engine.Engine
}

func (t *GetTool) Metadata() mcp.Tool {
	return mcp.NewTool("magicskills_get",
		mcp.WithDescription("Provides deep-dive access into a specific skill's core knowledge base. It supports version pinning and granular section retrieval (e.g., 'workflow', 'architecture', 'examples'). Use this to extract detailed instructions, logic rules, or best practices once a relevant skill has been identified."),
		mcp.WithString("name", mcp.Description("The name of the skill to retrieve"), mcp.Required()),
		mcp.WithString("section", mcp.Description("Optional granular section to retrieve")),
		mcp.WithString("version", mcp.Description("Optional minimum semver bound (e.g. 1.2.0)")),
	)
}

func (t *GetTool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	section := strings.ToLower(request.GetString("section", ""))
	versionBound := request.GetString("version", "")
	if name == "" {
		return mcp.NewToolResultError("missing 'name' argument"), nil
	}

	skill, ok := t.Engine.GetSkill(name)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("skill not found: %s", name)), nil
	}

	if versionBound != "" && skill.Metadata.Version != "" {
		vBound := versionBound
		vSkill := skill.Metadata.Version
		if !strings.HasPrefix(vBound, "v") {
			vBound = "v" + vBound
		}
		if !strings.HasPrefix(vSkill, "v") {
			vSkill = "v" + vSkill
		}
		if semver.IsValid(vBound) && semver.IsValid(vSkill) {
			if semver.Compare(vSkill, vBound) < 0 {
				return mcp.NewToolResultError(fmt.Sprintf("skill version %s is older than requested bound %s", skill.Metadata.Version, versionBound)), nil
			}
		}
	}

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
			return mcp.NewToolResultText(fmt.Sprintf("### %s: %s\n\n%s", name, section, engine.Densify(content))), nil
		}
	}

	return newHybridResult(skill), nil
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
			mcp.TextContent{Type: "text", Text: string(metaJSON)},
			mcp.TextContent{Type: "text", Text: skill.Digest},
		},
	}
}

// Register registers retrieval tools with the global registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&GetTool{Engine: eng})
}
