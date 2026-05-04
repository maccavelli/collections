// Package workspace provides functionality for the workspace subsystem.
package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/modfile"
)

// Info holds the metadata for a Go workspace/module context.
type Info struct {
	AbsPath       string // Input absolute path (physically resolved)
	ModuleRoot    string // Nearest go.mod or go.work directory
	ModuleName    string // Module name (empty if Workspace)
	RelativePkg   string // Package relative to ModuleRoot (e.g. ./internal/loader)
	IsModule      bool   // True if within a Go module
	IsWorkspace   bool   // True if using go.work
	DiscoveryType string // "go.mod", "go.work", "vcs", "fallback"
}

const (
	DiscoveryGoMod    = "go.mod"
	DiscoveryGoWork   = "go.work"
	DiscoveryVCS      = "vcs"
	DiscoveryFallback = "fallback"
)

// ModuleInfo represents the JSON output from 'go list -m -json'.
type ModuleInfo struct {
	Path string `json:"Path"`
	Dir  string `json:"Dir"`
}

// PackageInfo represents the JSON output from 'go list -json .'.
type PackageInfo struct {
	ImportPath string      `json:"ImportPath"`
	Module     *ModuleInfo `json:"Module"`
	Dir        string      `json:"Dir"`
}

// Discover takes an input path and resolves its workspace context using a robust tiered approach.
func Discover(ctx context.Context, inputPath string) (*Info, error) {
	// 1. Resolve absolute physical path (follow symlinks)
	abs, err := resolveAbs(inputPath)
	if err != nil {
		return nil, err
	}
	slog.Info("discovering workspace", "input", inputPath, "abs", abs)

	dir := abs
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}

	// 2. Try the Go environment directly (fast and reliable)
	if info, err := discoverViaEnv(ctx, abs, dir); err == nil && info != nil {
		return info, nil
	}

	// 3. Fallback: Manual search for markers with VCS boundary
	if info, err := discoverViaMarkers(ctx, abs, dir); err == nil && info != nil {
		return info, nil
	}

	// 4. Ultimate Fallback: No project found
	return &Info{
		AbsPath:       abs,
		ModuleRoot:    dir,
		IsModule:      false,
		RelativePkg:   ".",
		DiscoveryType: DiscoveryFallback,
	}, nil
}

func resolveAbs(inputPath string) (string, error) {
	abs, err := filepath.Abs(inputPath)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	realPath, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return realPath, nil
	}
	return abs, nil
}

func discoverViaEnv(ctx context.Context, abs, dir string) (*Info, error) {
	env, err := getGoEnv(ctx, dir)
	if err != nil {
		return nil, err
	}

	if env.GOWORK != "" && env.GOWORK != "off" && env.GOWORK != "null" {
		root := filepath.Dir(env.GOWORK)
		return &Info{
			AbsPath:       abs,
			ModuleRoot:    root,
			RelativePkg:   "./" + getRel(root, dir),
			IsModule:      true,
			IsWorkspace:   true,
			DiscoveryType: DiscoveryGoWork,
		}, nil
	}
	if env.GOMOD != "" && env.GOMOD != os.DevNull && env.GOMOD != "null" {
		root := filepath.Dir(env.GOMOD)
		name := getModuleName(ctx, root)
		return &Info{
			AbsPath:       abs,
			ModuleRoot:    root,
			ModuleName:    name,
			RelativePkg:   "./" + getRel(root, dir),
			IsModule:      true,
			DiscoveryType: DiscoveryGoMod,
		}, nil
	}
	return nil, fmt.Errorf("no env marker found")
}

func discoverViaMarkers(ctx context.Context, abs, dir string) (*Info, error) {
	root := dir
	// If path doesn't exist, traverse up from an existing parent
	for root != "" {
		if _, err := os.Stat(root); err == nil {
			break
		}
		root = filepath.Dir(root)
		if root == filepath.Dir(root) {
			break
		}
	}

	for root != "" {
		// Priority 1: go.work
		if _, err := os.Stat(filepath.Join(root, "go.work")); err == nil {
			return &Info{
				AbsPath:       abs,
				ModuleRoot:    root,
				IsModule:      true,
				IsWorkspace:   true,
				RelativePkg:   "./" + getRel(root, abs),
				DiscoveryType: DiscoveryGoWork,
			}, nil
		}
		// Priority 2: go.mod
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return &Info{
				AbsPath:       abs,
				ModuleRoot:    root,
				ModuleName:    getModuleName(ctx, root),
				IsModule:      true,
				RelativePkg:   "./" + getRel(root, abs),
				DiscoveryType: DiscoveryGoMod,
			}, nil
		}
		// Priority 3: VCS boundary
		if isVCSRoot(root) {
			return &Info{
				AbsPath:       abs,
				ModuleRoot:    root,
				IsModule:      false,
				RelativePkg:   "./" + getRel(root, abs),
				DiscoveryType: DiscoveryVCS,
			}, nil
		}

		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	return nil, fmt.Errorf("no marker found")
}

type goEnv struct {
	GOMOD  string
	GOWORK string
}

var (
	goEnvCache = make(map[string]*goEnv)
	goEnvMutex sync.RWMutex
)

func getGoEnv(ctx context.Context, dir string) (*goEnv, error) {
	goEnvMutex.RLock()
	if env, ok := goEnvCache[dir]; ok {
		goEnvMutex.RUnlock()
		return env, nil
	}
	goEnvMutex.RUnlock()

	tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(tctx, "go", "env", "-json", "GOMOD", "GOWORK")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var env goEnv
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, err
	}

	goEnvMutex.Lock()
	goEnvCache[dir] = &env
	goEnvMutex.Unlock()

	return &env, nil
}

func getModuleName(ctx context.Context, root string) string {
	modPath := filepath.Join(root, "go.mod")
	data, err := os.ReadFile(modPath)
	if err != nil {
		return ""
	}

	f, err := modfile.Parse(modPath, data, nil)
	if err != nil || f == nil || f.Module == nil || f.Module.Mod.Path == "" {
		return ""
	}

	return f.Module.Mod.Path
}

func isVCSRoot(root string) bool {
	markers := []string{".git", ".hg", ".svn"}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(root, m)); err == nil {
			return true
		}
	}
	return false
}

func getRel(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return ""
	}
	return rel
}

// ResolvePattern normalizes a Go pattern based on the workspace context.
func (i *Info) ResolvePattern(pattern string) string {
	if pattern == "" || pattern == "." {
		return i.RelativePkg
	}
	if strings.HasSuffix(pattern, "...") && i.RelativePkg != "." {
		if i.RelativePkg == "./" {
			return "./..."
		}
		return filepath.Join(i.RelativePkg, "...")
	}
	return pattern
}
