package handler

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-magicskills/internal/engine"
)

const (
	logBufferLimit = 1024 * 512 // 512KB
	logTrimTarget  = 256 * 1024 // 256KB
)

// LogBuffer implements io.Writer with ring-buffer semantics for slog
type LogBuffer struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	n, err = lb.buf.Write(p)
	if err != nil {
		return n, err
	}

	if lb.buf.Len() > logBufferLimit {
		data := lb.buf.Bytes()
		trimPoint := len(data) - logTrimTarget
		if idx := bytes.IndexByte(data[trimPoint:], '\n'); idx >= 0 {
			trimPoint += idx + 1
		}
		newData := make([]byte, len(data)-trimPoint)
		copy(newData, data[trimPoint:])
		lb.buf.Reset()
		_, _ = lb.buf.Write(newData)
	}
	return n, nil
}

func (lb *LogBuffer) String() string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.buf.String()
}

type MagicSkillsHandler struct {
	Engine *engine.Engine
	Logs   *LogBuffer
}

func (h *MagicSkillsHandler) HandleGetLogs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(h.Logs.String()), nil
}

func (h *MagicSkillsHandler) HandleSummarize(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("missing 'name' argument"), nil
	}

	summary, ok := h.Engine.Summarize(name)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("skill not found: %s", name)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("# Summary/Directive: %s\n\n%s", name, summary)), nil
}

func (h *MagicSkillsHandler) HandleListResources(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	resources := []mcp.Resource{
		mcp.NewResource("magicskills://status", "Skill Status Dashboard",
			mcp.WithResourceDescription("Overview of currently indexed and active skills."),
			mcp.WithMIMEType("text/markdown"),
		),
	}
	return &mcp.ListResourcesResult{Resources: resources}, nil
}

func (h *MagicSkillsHandler) HandleReadResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	if request.Params.URI == "magicskills://status" {
		var b strings.Builder
		b.Grow(512)
		b.WriteString("# MagicSkills Dashboard\n\n")
		b.WriteString(fmt.Sprintf("Total Skills Indexed: %d\n\n", len(h.Engine.Skills)))
		b.WriteString("## Available Skills\n")
		
		for s := range h.Engine.AllSkills() {
			b.WriteString(fmt.Sprintf("- **%s** (v%s)\n", s.Metadata.Name, s.Metadata.Version))
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "text/markdown",
				Text:     b.String(),
			},
		}, nil
	}
	return nil, fmt.Errorf("resource not found: %s", request.Params.URI)
}

func (h *MagicSkillsHandler) HandleGetSkill(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("missing 'name' argument"), nil
	}

	skill, ok := h.Engine.GetSkill(name)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("skill not found: %s", name)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("# Directive: %s\n\n%s", skill.Metadata.Description, skill.Sections["full"])), nil
}

func (h *MagicSkillsHandler) HandleListSkills(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var b strings.Builder
	b.Grow(1024)
	b.WriteString("Available MagicSkills Index:\n\n")

	for skill := range h.Engine.AllSkills() {
		b.WriteString(fmt.Sprintf("- **%s**: %s (v%s)\n", skill.Metadata.Name, skill.Metadata.Description, skill.Metadata.Version))
	}

	return mcp.NewToolResultText(b.String()), nil
}

func (h *MagicSkillsHandler) HandleGetSection(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	section := strings.ToLower(request.GetString("section", "workflow"))

	if name == "" {
		return mcp.NewToolResultError("missing 'name' argument"), nil
	}

	skill, ok := h.Engine.GetSkill(name)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("skill not found: %s", name)), nil
	}

	content, ok := skill.Sections[section]
	if !ok {
		for k, v := range skill.Sections {
			if strings.Contains(k, section) {
				content = v
				break
			}
		}
		if content == "" {
			return mcp.NewToolResultError(fmt.Sprintf("section '%s' not found in skill '%s'", section, name)), nil
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("# Skill: %s (%s)\n\n%s", name, section, content)), nil
}

func (h *MagicSkillsHandler) HandleBootstrapTask(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("missing 'name' argument"), nil
	}

	skill, ok := h.Engine.GetSkill(name)
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

func (h *MagicSkillsHandler) HandleMatchSkills(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	intent := request.GetString("intent", "")
	if intent == "" {
		return mcp.NewToolResultError("missing 'intent' argument"), nil
	}

	matches := h.Engine.MatchSkills(intent)
	if len(matches) == 0 {
		return mcp.NewToolResultText("No matching skills found for your intent."), nil
	}

	var b strings.Builder
	b.Grow(len(matches) * 64)
	b.WriteString(fmt.Sprintf("Skills matching intent '%s' (ordered by relevance):\n\n", intent))
	for _, m := range matches {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", m.Metadata.Name, m.Metadata.Description))
	}
	return mcp.NewToolResultText(b.String()), nil
}
