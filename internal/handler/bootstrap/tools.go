package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/registry"
)

// BootstrapTool implements the magicskills_bootstrap tool.
type BootstrapTool struct {
	Engine *engine.Engine
}

func (t *BootstrapTool) Name() string { return "magicskills_bootstrap" }

type BootstrapInput struct {
	Name string `json:"name" jsonschema:"The skill name"`
}

func (t *BootstrapTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "PLAN SYNTHESIS: Call this to synthesize an actionable markdown task checklist for the current goal. Required before starting any development or refactoring work to ensure progress tracking. Cascades to magicskills_validate_deps.",
	}, t.Handle)
}

func (t *BootstrapTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input BootstrapInput) (*mcp.CallToolResult, any, error) {
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

	content, ok := skill.Sections["workflow"]
	if !ok {
		content, ok = skill.Sections["checklist"]
	}
	if !ok {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "No workflow or checklist found in skill."}},
		}, nil, nil
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
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Found workflow section but no bullet points to bootstrap."}},
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "# Tasks\n\n" + checklist.String()}},
	}, nil, nil
}

// Register registers bootstrap tools with the global registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&BootstrapTool{Engine: eng})
}

