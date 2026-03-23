package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/mod/semver"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/models"
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
		_, err = lb.buf.Write(newData)
		if err != nil {
			return 0, fmt.Errorf("failed to rewrite buffer: %w", err)
		}
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

// HandleReadResource handles both status and logs resources.
func (h *MagicSkillsHandler) HandleReadResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	switch request.Params.URI {
	case "magicskills://status":
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
	case "magicskills://logs":
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "text/plain",
				Text:     h.Logs.String(),
			},
		}, nil
	default:
		return nil, fmt.Errorf("resource not found: %s", request.Params.URI)
	}
}

func (h *MagicSkillsHandler) HandleListResources(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	resources := []mcp.Resource{
		mcp.NewResource("magicskills://status", "Skill Status Dashboard",
			mcp.WithResourceDescription("Overview of currently indexed and active skills."),
			mcp.WithMIMEType("text/markdown"),
		),
		mcp.NewResource("magicskills://logs", "Internal Logs",
			mcp.WithResourceDescription("Internal server logs for monitoring."),
			mcp.WithMIMEType("text/plain"),
		),
	}
	return &mcp.ListResourcesResult{Resources: resources}, nil
}

func (h *MagicSkillsHandler) HandleGetSkill(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	section := strings.ToLower(request.GetString("section", ""))
	versionBound := request.GetString("version", "")
	if name == "" {
		return mcp.NewToolResultError("missing 'name' argument"), nil
	}

	skill, ok := h.Engine.GetSkill(name)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("skill not found: %s", name)), nil
	}

	// Semver boundary check if requested
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

	// If a section is requested, find it and return its densified form
	if section != "" {
		content, found := skill.Sections[section]
		if !found {
			// Substring search for section headings
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

func (h *MagicSkillsHandler) HandleListSkills(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var b strings.Builder
	b.Grow(1024)
	b.WriteString("Available MagicSkills Index:\n\n")

	for skill := range h.Engine.AllSkills() {
		b.WriteString(fmt.Sprintf("- **%s**: %s (v%s)\n", skill.Metadata.Name, skill.Metadata.Description, skill.Metadata.Version))
	}

	return mcp.NewToolResultText(b.String()), nil
}

// HandleValidateDeps reads the skill's Requirements and checks if host binaries exist.
func (h *MagicSkillsHandler) HandleValidateDeps(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("missing 'name' argument"), nil
	}

	skill, ok := h.Engine.GetSkill(name)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("skill not found: %s", name)), nil
	}

	if len(skill.Metadata.Requirements) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("Skill '%s' has no specific host binary requirements.", name)), nil
	}

	var missing []string
	var found []string
	for _, req := range skill.Metadata.Requirements {
		if _, err := exec.LookPath(req); err != nil {
			missing = append(missing, req)
		} else {
			found = append(found, req)
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Dependencies Check for '%s':\n", name))
	if len(missing) > 0 {
		b.WriteString("\n**MISSING BINARIES**:\n")
		for _, m := range missing {
			b.WriteString(fmt.Sprintf("- %s\n", m))
		}
	}
	if len(found) > 0 {
		b.WriteString("\n**FOUND BINARIES**:\n")
		for _, f := range found {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}

	if len(missing) > 0 {
		return mcp.NewToolResultText(b.String()), nil
	}
	b.WriteString("\nAll dependencies met. Ready to execute.")
	return mcp.NewToolResultText(b.String()), nil
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

	// Hybrid approach: List of matches + Dense Digest for the top match
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

func newHybridResult(skill *models.Skill) *mcp.CallToolResult {
	// Structured JSON 2.0 Metadata
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
