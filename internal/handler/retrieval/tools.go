package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/external"
	"mcp-server-magicskills/internal/models"
	"mcp-server-magicskills/internal/registry"
	"mcp-server-magicskills/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/mod/semver"
)

// GetTool implements the magicskills_get tool.
type GetTool struct {
	Engine       *engine.Engine
	RecallClient *external.MCPClient
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
		Description: "[DIRECTIVE: Rule Extraction] Fetch, read, and retrieve the full fundamental Markdown source code and logic directives natively. Keywords: read-skill, obtain, pull-rules, exact-markdown, read-file",
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
		RecallStandards []map[string]any `json:"recall_standards,omitempty"`
	}{
		Skill: skill,
	}

	// Standards-Aware Enrichment (orchestrator mode only)
	if t.RecallClient != nil && t.RecallClient.RecallEnabled() {
		searchArgs := map[string]any{
			"query": skill.Metadata.Name,
			"limit": 3,
		}
		res := t.RecallClient.CallDatabaseTool(ctx, "search", appendNamespace(searchArgs, "standards"))
		if res != "" {
			var searchRes struct {
				Entries []map[string]any `json:"entries"`
			}
			if json.Unmarshal([]byte(res), &searchRes) == nil {
				output.RecallStandards = searchRes.Entries
			}
		}
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
func Register(eng *engine.Engine, cl *external.MCPClient) {
	registry.Global.Register(&GetTool{Engine: eng, RecallClient: cl})
}

func appendNamespace(m map[string]any, ns string) map[string]any {
	if m == nil {
		m = make(map[string]any)
	}
	m["namespace"] = ns
	return m
}
