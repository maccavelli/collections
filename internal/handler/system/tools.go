package system

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/handler"
	"mcp-server-magicskills/internal/registry"
	"mcp-server-magicskills/internal/scanner"
	"mcp-server-magicskills/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ValidateDepsTool implements the magicskills_validate_deps tool.
type ValidateDepsTool struct {
	Engine *engine.Engine
}

func (t *ValidateDepsTool) Name() string { return "magicskills_validate_deps" }

// ValidateDepsInput defines the structural representation for the entity.
type ValidateDepsInput struct {
	Name string `json:"name" jsonschema:"The skill name"`
}

func (t *ValidateDepsTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Environment Sanity Check] Operates as an OS-level requisite verification to ensure all necessary binary constraints exist natively. Keywords: prerequisites, check-deps, verification, binaries [CONSTRAINT: After satisfaction, implement goal, then formally report efficacy.]",
	}, t.Handle)
}

func (t *ValidateDepsTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input ValidateDepsInput) (*mcp.CallToolResult, any, error) {
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

	var missing []string
	var found []string
	for _, req := range skill.Metadata.Requirements {
		if _, err := exec.LookPath(req); err != nil {
			missing = append(missing, req)
		} else {
			found = append(found, req)
		}
	}

	summary := fmt.Sprintf("All dependencies met for skill %s", input.Name)
	if len(missing) > 0 {
		summary = fmt.Sprintf("Missing dependencies for skill %s: %s", input.Name, strings.Join(missing, ", "))
	} else if len(skill.Metadata.Requirements) == 0 {
		summary = fmt.Sprintf("Skill %s has no binary requirements", input.Name)
	}

	return &mcp.CallToolResult{}, struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	}{
		Summary: summary,
		Data: map[string]any{
			"skill_name": input.Name,
			"found":      found,
			"missing":    missing,
		},
	}, nil
}

// AddRootTool implements the magicskills_add_root tool.
type AddRootTool struct {
	Scanner *scanner.Scanner
	Engine  *engine.Engine
}

func (t *AddRootTool) Name() string { return "magicskills_add_root" }

// AddRootInput defines the structural representation for the entity.
type AddRootInput struct {
	Path string `json:"path" jsonschema:"Absolute path to index"`
}

func (t *AddRootTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Ecosystem Extensibility] Dynamically adds a new absolute path directory to the indexing reach. Keywords: append, add-path, directory, discover, register [CONSTRAINT: Require absolute index enumeration immediately following.]",
	}, t.Handle)
}

func (t *AddRootTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input AddRootInput) (*mcp.CallToolResult, any, error) {
	if err := t.Engine.WaitReady(ctx); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("engine initialization aborted: %w", err))
		return res, nil, nil
	}
	if info, err := os.Stat(input.Path); err == nil && info.IsDir() {
		t.Scanner.Roots = append(t.Scanner.Roots, input.Path)
		files, err := t.Scanner.Discover(ctx)
		if err != nil {
			slog.Error("discovery error while adding manual root", "error", err)
			res := &mcp.CallToolResult{}
			res.SetError(fmt.Errorf("discovery error: %w", err))
			return res, nil, nil
		}
		if _, _, _, err := t.Engine.SyncDir(ctx, files); err != nil {
			slog.Error("ingestion error while adding manual root", "error", err)
			res := &mcp.CallToolResult{}
			res.SetError(fmt.Errorf("sync error: %w", err))
			return res, nil, nil
		}
		summary := fmt.Sprintf("Added and indexed root: %s", input.Path)
		return &mcp.CallToolResult{}, struct {
			Summary string `json:"summary"`
			Data    any    `json:"data"`
		}{
			Summary: summary,
			Data:    map[string]string{"path": input.Path},
		}, nil
	}
	res := &mcp.CallToolResult{}
	res.SetError(fmt.Errorf("invalid path: %s", input.Path))
	return res, nil, nil
}

// GetInternalLogsTool handles log retrieval.
type GetInternalLogsTool struct {
	Buffer *handler.LogBuffer
}

func (t *GetInternalLogsTool) Name() string { return "get_internal_logs" }

// LogsInput defines the structural representation for the entity.
type LogsInput struct {
	MaxLines int `json:"max_lines" jsonschema:"Max log lines to return (default 25)."`
}

func (t *GetInternalLogsTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Audit Streaming] Provides access to the server's internal diagnostic stream and audit trail exclusively for troubleshooting. Keywords: daemon-logs, traces, faults, underlying-errors, stdout",
	}, t.Handle)
}

func (t *GetInternalLogsTool) Handle(_ context.Context, req *mcp.CallToolRequest, input LogsInput) (*mcp.CallToolResult, any, error) {
	maxLines := 25
	if input.MaxLines > 0 {
		maxLines = input.MaxLines
	}

	logs := t.Buffer.String()
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: strings.Join(lines, "\n")}},
	}, nil, nil
}

// HealthTool implements the magicskills_health tool.
type HealthTool struct {
	Engine *engine.Engine
}

func (t *HealthTool) Name() string { return "magicskills_health" }

// HealthInput defines the structural representation for the entity.
type HealthInput struct{}

func (t *HealthTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Ecosystem Diagnostics] Fetch the list of degraded capabilities requiring manual refactoring or removal. Keywords: health, failures, degraded, broken, errors, maintenance",
	}, t.Handle)
}

func (t *HealthTool) Handle(ctx context.Context, _ *mcp.CallToolRequest, _ HealthInput) (*mcp.CallToolResult, any, error) {
	if err := t.Engine.WaitReady(ctx); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("engine initialization aborted: %w", err))
		return res, nil, nil
	}
	broken := t.Engine.GetBrokenSkills()

	summary := "All skills are healthy."
	if len(broken) > 0 {
		summary = fmt.Sprintf("Found %d degraded skills requiring refactoring.", len(broken))
	}

	return &mcp.CallToolResult{}, struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	}{
		Summary: summary,
		Data: map[string]any{
			"degraded_skills": broken,
		},
	}, nil
}

// Register registers system tools with the global registry.
func Register(eng *engine.Engine, scn *scanner.Scanner, lb *handler.LogBuffer) {
	registry.Global.Register(&ValidateDepsTool{Engine: eng})
	registry.Global.Register(&AddRootTool{Scanner: scn, Engine: eng})
	registry.Global.Register(&GetInternalLogsTool{Buffer: lb})
	registry.Global.Register(&HealthTool{Engine: eng})
}
