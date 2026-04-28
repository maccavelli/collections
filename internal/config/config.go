package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Config holds the application-wide configuration for MagicSkills.
type Config struct {
	Version   string
	Roots     []string
	RecallURL string // Canonical endpoint for mcp-server-recall (HTTP-streaming)
}

const (
	// Logging: get_internal_logs buffer limits
	LogBufferLimit  = 1024 * 1024 // 1MB max log buffer
	LogTrimTarget   = 512 * 1024  // 512KB trim target
	DefaultLogLines = 25          // Default lines returned by get_internal_logs
)

// ResolveRoots canonicalizes and discovers skill roots from environment and default paths.
func ResolveRoots() []string {
	var roots []string
	homePath, err := os.UserHomeDir()
	if err != nil {
		// Suppress error, fall back to empty home
		homePath = ""
	}

	if val := os.Getenv("MAGIC_SKILLS_PATH"); val != "" {
		roots = append(roots, filepath.SplitList(val)...)
	}

	candidates := []string{
		filepath.Join(homePath, ".gemini/skills"),
		filepath.Join(homePath, ".gemini/antigravity/skills"),
		filepath.Join(homePath, ".antigravity/skills"),
		filepath.Join(homePath, ".agents/skills"),
		filepath.Join(homePath, ".agent/skills"),
		filepath.Join(homePath, ".claude/rules"),
		filepath.Join(homePath, ".cursor/rules"),
		filepath.Join(homePath, ".copilot/skills"),
		filepath.Join(homePath, "gitrepos/saxsmith-global-context/.agent/skills"),
	}

	for _, c := range candidates {
		if c == "" {
			continue
		}
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			roots = append(roots, c)
		}
	}

	// Deduplicate roots
	seen := make(map[string]struct{})
	unique := make([]string, 0, len(roots))
	for _, r := range roots {
		abs, err := filepath.Abs(r)
		if err != nil {
			abs = r
		}
		abs = strings.TrimSuffix(abs, "/")
		if _, ok := seen[abs]; !ok {
			seen[abs] = struct{}{}
			unique = append(unique, abs)
		}
	}
	return unique
}

// ResolveDataDir returns the path for the persistent BadgerDB.
func ResolveDataDir() string {
	if val := os.Getenv("MAGIC_SKILLS_DATA_DIR"); val != "" {
		return val
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "/tmp/mcp-server-magicskills-data"
	}
	return filepath.Join(configDir, "mcp-server-magicskills")
}

// ResolveRecallURL identifies the MCP recall server endpoint for cross-server talk.
func ResolveRecallURL() string {
	if val := os.Getenv("MCP_RECALL_URL"); val != "" {
		return val
	}
	if val := os.Getenv("MCP_API_URL"); val != "" {
		return val
	}
	return "http://localhost:8080/sse"
}

// ResolveRedactionPattern fetches the standard redaction regex for logs or defaults it.
func ResolveRedactionPattern() string {
	if val := os.Getenv("MAGIC_SKILLS_REDACTION_PATTERN"); val != "" {
		return val
	}
	return `(?i)(token_|sk_|key_|secret_)[a-zA-Z0-9_-]+`
}
