package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Config holds the application-wide configuration for MagicSkills.
type Config struct {
	Version string
	Roots   []string
}

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
