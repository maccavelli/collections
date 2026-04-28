// Package pathutil provides cross-platform path normalization, validation,
// and security enforcement for the filesystem MCP server.
package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Manager holds the set of allowed directories and provides thread-safe
// path validation with symlink resolution.
type Manager struct {
	mu      sync.RWMutex
	allowed []string
}

// NewManager creates a Manager with the given allowed directories.
// Each directory is normalized and, if possible, resolved to its real path.
func NewManager(dirs []string) *Manager {
	m := &Manager{}
	m.SetAllowed(dirs)
	return m
}

// SetAllowed replaces the allowed directory list. Both the original and
// resolved (symlink-followed) forms are stored so that macOS /tmp →
// /private/tmp style aliases work correctly.
func (m *Manager) SetAllowed(dirs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	seen := make(map[string]struct{}, len(dirs)*2)
	var result []string

	for _, dir := range dirs {
		expanded := ExpandHome(dir)
		abs, err := filepath.Abs(expanded)
		if err != nil {
			continue
		}
		norm := NormalizePath(abs)
		if _, ok := seen[norm]; !ok {
			seen[norm] = struct{}{}
			result = append(result, norm)
		}

		// Also store the symlink-resolved form.
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			continue
		}
		normResolved := NormalizePath(resolved)
		if _, ok := seen[normResolved]; !ok {
			seen[normResolved] = struct{}{}
			result = append(result, normResolved)
		}
	}
	m.allowed = result
}

// Allowed returns a copy of the current allowed directories.
func (m *Manager) Allowed() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, len(m.allowed))
	copy(out, m.allowed)
	return out
}

// ValidatePath checks that requestedPath is within an allowed directory,
// resolving symlinks for security. It returns the real (resolved) path.
func (m *Manager) ValidatePath(requestedPath string) (string, error) {
	expanded := ExpandHome(requestedPath)
	var abs string
	if filepath.IsAbs(expanded) {
		abs = filepath.Clean(expanded)
	} else {
		abs = m.resolveRelative(expanded)
	}

	norm := NormalizePath(abs)

	m.mu.RLock()
	allowed := m.allowed
	m.mu.RUnlock()

	if !IsPathWithinAllowed(norm, allowed) {
		return "", fmt.Errorf("access denied - path outside allowed directories: %s", abs)
	}

	// Resolve symlinks to prevent symlink-based escapes.
	realPath, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		// For new files, verify the parent exists and is allowed.
		parentDir := filepath.Dir(abs)
		realParent, err2 := filepath.EvalSymlinks(parentDir)
		if err2 != nil {
			return "", fmt.Errorf("parent directory does not exist: %s", parentDir)
		}
		normParent := NormalizePath(realParent)
		if !IsPathWithinAllowed(normParent, allowed) {
			return "", fmt.Errorf("access denied - parent directory outside allowed directories: %s", realParent)
		}
		return abs, nil
	}

	normReal := NormalizePath(realPath)
	if !IsPathWithinAllowed(normReal, allowed) {
		return "", fmt.Errorf("access denied - symlink target outside allowed directories: %s", realPath)
	}
	return realPath, nil
}

// resolveRelative resolves a relative path against the first matching
// allowed directory.
func (m *Manager) resolveRelative(relPath string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, dir := range m.allowed {
		candidate := filepath.Join(dir, relPath)
		norm := NormalizePath(candidate)
		if IsPathWithinAllowed(norm, m.allowed) {
			return candidate
		}
	}
	if len(m.allowed) > 0 {
		return filepath.Join(m.allowed[0], relPath)
	}
	abs, err := filepath.Abs(relPath)
	if err != nil {
		return relPath
	}
	return abs
}

// ExpandHome expands a leading ~ to the user's home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[1:])
	}
	return p
}

// NormalizePath produces a cleaned, absolute-looking path with
// platform-appropriate separators and casing.
func NormalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"'`)

	// On Windows, capitalize drive letters.
	if runtime.GOOS == "windows" && len(p) >= 2 && p[1] == ':' {
		p = strings.ToUpper(p[:1]) + p[1:]
	}

	cleaned := filepath.Clean(p)
	return cleaned
}

// IsPathWithinAllowed returns true if absPath is equal to or a child of
// any directory in allowed.
func IsPathWithinAllowed(absPath string, allowed []string) bool {
	if absPath == "" || len(allowed) == 0 {
		return false
	}
	// Reject null bytes.
	if strings.ContainsRune(absPath, 0) {
		return false
	}

	for _, dir := range allowed {
		if dir == "" || strings.ContainsRune(dir, 0) {
			continue
		}
		if absPath == dir {
			return true
		}
		prefix := dir
		if !strings.HasSuffix(prefix, string(filepath.Separator)) {
			prefix += string(filepath.Separator)
		}
		if strings.HasPrefix(absPath, prefix) {
			return true
		}
	}
	return false
}
