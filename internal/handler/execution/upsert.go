package execution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/scanner"
	"mcp-server-magicskills/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UpsertTool defines the structural representation for the entity.
type UpsertTool struct {
	Engine *engine.Engine
}

func (t *UpsertTool) Name() string { return "magicskills_upsert" }

// UpsertInput defines the structural representation for the entity.
type UpsertInput struct {
	Name          string   `json:"name" jsonschema:"The formal name of the skill"`
	Description   string   `json:"description" jsonschema:"Brief description of what the skill does"`
	ContextDomain string   `json:"context_domain,omitempty" jsonschema:"Optional domain classification (e.g. 'go', 'python')"`
	Tags          []string `json:"tags,omitempty" jsonschema:"Tags for the skill"`
	Content       string   `json:"content" jsonschema:"The raw markdown content of the skill, excluding YAML frontmatter"`
}

func (t *UpsertTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Engine Mutation] Write, author, persist, and update autonomous formatting definitions. Directly injects into the global search index. Keywords: create, write, update, save, inject-logic, persist",
	}, t.Handle)
}

func (t *UpsertTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input UpsertInput) (*mcp.CallToolResult, any, error) {
	if err := t.Engine.WaitReady(ctx); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("engine initialization aborted: %w", err))
		return res, nil, nil
	}
	if input.Name == "" || input.Description == "" || input.Content == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("missing 'name', 'description', or 'content' fields"))
		return res, nil, nil
	}

	roots := scanner.FindProjectSkillsRoots()
	if len(roots) == 0 {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("no valid .agent/skills root directories found to upsert into"))
		return res, nil, nil
	}
	targetRoot := roots[0]

	dirName := strings.ReplaceAll(strings.ToLower(input.Name), " ", "-")
	targetDir := filepath.Join(targetRoot, dirName)
	if err := os.MkdirAll(targetDir, 0750); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to create skill directory: %w", err))
		return res, nil, nil
	}

	targetFile := filepath.Join(targetDir, "SKILL.md")

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", input.Name))
	sb.WriteString(fmt.Sprintf("description: %v\n", strings.ReplaceAll(strings.ReplaceAll(input.Description, "\n", " "), "\r", "")))
	if input.ContextDomain != "" {
		sb.WriteString(fmt.Sprintf("context_domain: %s\n", input.ContextDomain))
	}
	if len(input.Tags) > 0 {
		sb.WriteString("tags:\n")
		for _, tag := range input.Tags {
			sb.WriteString(fmt.Sprintf("  - %s\n", tag))
		}
	}
	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimSpace(input.Content))
	sb.WriteString("\n")

	if err := os.WriteFile(targetFile, []byte(sb.String()), 0600); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to write skill file: %w", err))
		return res, nil, nil
	}

	if err := t.Engine.IngestSingle(ctx, targetFile); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to ingest skill natively: %w", err))
		return res, nil, nil
	}

	return &mcp.CallToolResult{}, struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	}{
		Summary: fmt.Sprintf("Successfully upserted and indexed %s", input.Name),
		Data: map[string]any{
			"path":   targetFile,
			"target": targetRoot,
		},
	}, nil
}
