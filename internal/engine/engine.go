// Package engine implements the core reasoning logic for the
// Socratic brainstorming MCP server. It provides heuristic-based
// analysis for project discovery, design stress-testing, quality
// auditing, adversarial review, and architectural decision
// capture.
package engine

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
)

// maxWalkDepth limits how deep AnalyzeDiscovery recurses
// into the project tree to prevent runaway traversals.
const maxWalkDepth = 3

// RecallClient defines the subset of MCPClient methods required by the engine.
type RecallClient interface {
	RecallEnabled() bool
	CallDatabaseTool(ctx context.Context, toolName string, arguments map[string]any) string
	AggregateSessionFromRecall(ctx context.Context, serverID, projectID string) (map[string]any, error)
	SaveSession(ctx context.Context, sessionID, projectID string, payload any) error
}

// skipDirs contains directory names that are always excluded
// from filesystem analysis.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"__pycache__":  true,
	".venv":        true,
}

// Engine facilitates the Socratic brainstorming process.
type Engine struct {
	ProjectRoot    string
	mu             sync.RWMutex
	ExternalClient RecallClient
	mcpSession     *mcp.ServerSession
	DB             *buntdb.DB
}

// NewEngine creates a new Engine rooted at the given
// directory.
func NewEngine(root string, db *buntdb.DB) *Engine {
	return &Engine{
		ProjectRoot: root,
		DB:          db,
	}
}

// DBEntries returns the count of persistent entries in BuntDB.
func (e *Engine) DBEntries() int {
	if e == nil || e.DB == nil {
		return 0
	}
	var n int
	_ = e.DB.View(func(tx *buntdb.Tx) error {
		n, _ = tx.Len()
		return nil
	})
	return n
}

// SetExternalClient injects the RecallClient for cross-server API tools (e.g. Recall).
func (e *Engine) SetExternalClient(c RecallClient) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ExternalClient = c
}

// SetSession performs the SetSession operation.
func (e *Engine) SetSession(s *mcp.ServerSession) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mcpSession = s
}

// Session performs the Session operation.
func (e *Engine) Session() *mcp.ServerSession {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mcpSession
}

// search helper removed; EnsureRecallCache uses CallDatabaseTool directly

// projectSentinels defines markers searched in priority order
// to identify the project root boundary.
var projectSentinels = []struct {
	name string
	kind string
}{
	{"go.work", "workspace"},
	{"go.mod", "module"},
	{".git", "vcs"},
	{".hg", "vcs"},
	{".svn", "vcs"},
}

// findProjectRoot walks up from startDir searching for
// sentinel files that indicate a project root boundary.
func findProjectRoot(startDir string) (string, string) {
	dir := startDir
	for {
		for _, s := range projectSentinels {
			if _, err := os.Stat(filepath.Join(dir, s.name)); err == nil {
				return dir, s.kind
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return startDir, "fallback"
}

// ResolvePath returns the project root for the given path by
// walking up the directory tree to find sentinel files. Falls
// back to the engine's ProjectRoot if path is empty.
func (e *Engine) ResolvePath(path string) string {
	if path == "" {
		return e.ProjectRoot
	}
	abs := path
	if !filepath.IsAbs(path) {
		abs = filepath.Join(e.ProjectRoot, path)
	}
	root, kind := findProjectRoot(abs)
	slog.Debug("resolved project root", "input", path, "root", root, "via", kind)
	return root
}

// fileExists returns true if a path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
