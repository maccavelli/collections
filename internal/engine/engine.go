// Package engine implements the core reasoning logic for the
// Socratic brainstorming MCP server. It provides heuristic-based
// analysis for project discovery, design stress-testing, quality
// auditing, adversarial review, and architectural decision
// capture.
package engine

import (
	"os"
	"path/filepath"

	"mcp-server-brainstorm/internal/analysis"
)

// maxWalkDepth limits how deep AnalyzeDiscovery recurses
// into the project tree to prevent runaway traversals.
const maxWalkDepth = 3

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
	ProjectRoot string
	inspector   *analysis.Inspector
}

// NewEngine creates a new Engine rooted at the given
// directory.
func NewEngine(root string) *Engine {
	return &Engine{
		ProjectRoot: root,
		inspector:   analysis.NewInspector(),
	}
}

// ResolvePath returns the absolute path provided, or the
// project root if empty.
func (e *Engine) ResolvePath(path string) string {
	if path == "" {
		return e.ProjectRoot
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(e.ProjectRoot, path)
}

// fileExists returns true if a path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
