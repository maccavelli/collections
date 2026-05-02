package sync

import (
	"context"
	"fmt"
	"mcp-server-magicskills/internal/util"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/registry"
	"mcp-server-magicskills/internal/scanner"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SyncTool implements the magicskills_sync_skills tool.
type SyncTool struct {
	Engine  *engine.Engine
	Scanner *scanner.Scanner
}

func (t *SyncTool) Name() string { return "magicskills_sync_skills" }

// SyncInput defines the structural representation for the entity.
type SyncInput struct{}

func (t *SyncTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Cache Re-Alignment] Refresh, reload, and globally synchronize the master file directory calculating checksum differentials. Keywords: refresh, force-sync, update-cache, delta, sha-256",
	}, t.Handle)
}

func (t *SyncTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input SyncInput) (*mcp.CallToolResult, any, error) {
	if err := t.Engine.WaitReady(ctx); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("engine initialization aborted: %w", err))
		return res, nil, nil
	}
	files, err := t.Scanner.Discover(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("discovery failed: %w", err))
		return res, nil, nil
	}

	added, updated, deleted, err := t.Engine.SyncDir(ctx, files)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("sync failed: %w", err))
		return res, nil, nil
	}

	summary := fmt.Sprintf("Successfully synced skills: %d added, %d updated, %d deleted.", added, updated, deleted)
	if added == 0 && updated == 0 && deleted == 0 {
		summary = "Skills are already up to date. No changes detected."
	}

	return &mcp.CallToolResult{}, struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	}{
		Summary: summary,
		Data: map[string]any{
			"added":   added,
			"updated": updated,
			"deleted": deleted,
			"total":   len(files),
		},
	}, nil
}

// Register registers sync tools with the global registry.
func Register(eng *engine.Engine, scn *scanner.Scanner) {
	registry.Global.Register(&SyncTool{Engine: eng, Scanner: scn})
}
