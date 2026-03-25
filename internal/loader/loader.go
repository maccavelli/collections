package loader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"log/slog"
	"strings"

	"mcp-server-go-refactor/internal/runner"
	"mcp-server-go-refactor/internal/workspace"

	"golang.org/x/tools/go/packages"
)

// DefaultMode provides the standard set of flags needed for most AST analysis.
const DefaultMode = packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedImports

// ResolveDir identifies the working directory and pattern to use for a given package path.
// Deprecated: use LoadWorkspace instead for better metadata.
func ResolveDir(pkgPath string) (dir string, pattern string) {
	p := strings.TrimSpace(pkgPath)
	recursive := false
	if strings.HasSuffix(p, "...") {
		recursive = true
		p = strings.TrimSuffix(p, "...")
		p = strings.TrimSuffix(p, "/")
	}

	pattern = pkgPath
	if filepath.IsAbs(p) {
		info, err := os.Stat(p)
		if err == nil {
			if info.IsDir() {
				dir = p
				if recursive {
					pattern = "./..."
				} else {
					pattern = "."
				}
			} else {
				dir = filepath.Dir(p)
				pattern = "."
			}
		}
	}
	return dir, pattern
}

// Result holds the resolved workspace info and a hardened runner.
type Result struct {
	Workspace *workspace.Info
	Runner    *runner.Runner
	Pattern   string
}

// Discover takes an input path and resolves its workspace context using 'go list'.
// Discover takes an input path and resolves its workspace context using 'go list'.
func Discover(ctx context.Context, pkgPath string) (*Result, error) {
	p, recursive := parsePath(pkgPath)
	abs, isLocal := resolveAbsPath(p)

	// 1. Try to get module/package info via 'go list'
	res, serr, err := discoverViaGoList(ctx, p, abs, isLocal, recursive)
	if err == nil {
		return res, nil
	}

	// 2. Fallback for non-local paths: Search for neighbors (siblings)
	if !isLocal {
		if res, err := tryNeighborDiscovery(ctx, pkgPath); err == nil && res != nil {
			return res, nil
		}
	}
	
	// 3. Fallback for local paths if 'go list' failed
	if isLocal {
		if ws, err := workspace.Discover(ctx, p); err == nil {
			pattern := "."
			if recursive {
				pattern = "./..."
			}
			return &Result{
				Workspace: ws,
				Runner:    runner.New(ws.ModuleRoot),
				Pattern:   ws.ResolvePattern(pattern),
			}, nil
		}
	}

	return nil, fmt.Errorf("failed to discover workspace for %s: %s", pkgPath, serr)
}

func tryNeighborDiscovery(ctx context.Context, pkgPath string) (*Result, error) {
	p, recursive := parsePath(pkgPath)
	parts := strings.Split(p, "/")
	if len(parts) == 0 {
		return nil, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, nil
	}

	// Scan parents (up to 4 levels) to find siblings that look like the module
	curr := wd
	for i := 0; i < 4; i++ {
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}

		// Look for a neighbor directory whose name matches the first segment of the package path
		neighbor := filepath.Join(parent, parts[0])
		if info, err := os.Stat(neighbor); err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(neighbor, "go.mod")); err == nil {
				// We found a likely module mirror. Resolve to the absolute path.
				full := neighbor
				if len(parts) > 1 {
					full = filepath.Join(neighbor, filepath.Join(parts[1:]...))
				}
				if recursive {
					full += "/..."
				}
				slog.Info("resolved package via neighbor search", "pkg", pkgPath, "abs", full)
				return Discover(ctx, full)
			}
		}
		curr = parent
	}
	return nil, nil
}

func parsePath(pkgPath string) (p string, recursive bool) {
	p = strings.TrimSpace(pkgPath)
	recursive = strings.HasSuffix(p, "...")
	if recursive && p != "..." {
		p = strings.TrimSuffix(p, "...")
		p = strings.TrimSuffix(p, "/")
	}
	return p, recursive
}

func resolveAbsPath(p string) (abs string, isLocal bool) {
	var err error
	abs, err = filepath.Abs(p)
	if err != nil {
		// Fallback to input path if Abs fails, though this is rare on sane systems
		abs = p
	}

	if p == "all" || p == "..." {
		isLocal = true
		if a, err := filepath.Abs("."); err == nil {
			abs = a
		}
	} else if filepath.IsAbs(p) || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
		isLocal = true
	} else if _, err := os.Stat(p); err == nil {
		isLocal = true
	}
	return abs, isLocal
}

func discoverViaGoList(ctx context.Context, p, abs string, isLocal, recursive bool) (*Result, string, error) {
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var listArgs []string
	var dir string

	if isLocal {
		if info, err := os.Stat(abs); err == nil {
			if !info.IsDir() {
				dir = filepath.Dir(abs)
			} else {
				dir = abs
			}
			listArgs = []string{"list", "-json", "."}
		} else {
			listArgs = []string{"list", "-json", p}
			// Use nearest parent for CMD dir so go list runs in a module context
			curr := abs
			for curr != "" {
				if _, err := os.Stat(curr); err == nil {
					dir = curr
					break
				}
				curr = filepath.Dir(curr)
				if curr == filepath.Dir(curr) {
					break
				}
			}
		}
	} else {
		listArgs = []string{"list", "-json", p}
	}

	if p == "all" || p == "..." {
		listArgs = []string{"list", "-m", "-json", "main"}
	}
	
	cmd := exec.CommandContext(tctx, "go", listArgs...)
	if dir != "" {
		cmd.Dir = dir
	}

	var out, serr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &serr

	if err := cmd.Run(); err != nil {
		return nil, serr.String(), err
	}

	// Unmarshal and resolve result
	res, err := resolveResult(out.Bytes(), p, recursive)
	return res, serr.String(), err
}

func resolveResult(data []byte, p string, recursive bool) (*Result, error) {
	var pkg workspace.PackageInfo
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	mInfo := pkg.Module
	if mInfo == nil && p == "all" {
		var m workspace.ModuleInfo
		if err := json.Unmarshal(data, &m); err == nil {
			mInfo = &m
		}
	}

	if mInfo == nil {
		return nil, fmt.Errorf("no module info found")
	}

	rel := "."
	if pkg.Dir != "" && mInfo.Dir != "" {
		if r, err := filepath.Rel(mInfo.Dir, pkg.Dir); err == nil {
			if r == "." {
				rel = "./"
			} else {
				rel = "./" + r
			}
		}
	}

	wInfo := &workspace.Info{
		AbsPath:     pkg.Dir,
		ModuleRoot:  mInfo.Dir,
		ModuleName:  mInfo.Path,
		RelativePkg: rel,
		IsModule:    true,
	}

	pattern := rel
	if p == "all" {
		pattern = "all"
	} else if recursive {
		pattern = rel + "/..."
	}

	return &Result{
		Workspace: wInfo,
		Runner:    runner.New(wInfo.ModuleRoot),
		Pattern:   pattern,
	}, nil
}

// LoadPackages provides a robust way to load Go packages, handling absolute paths
// by correctly setting the working directory for the build system query tool.
func LoadPackages(ctx context.Context, pkgPath string, mode packages.LoadMode) ([]*packages.Package, error) {
	res, err := Discover(ctx, pkgPath)
	if err != nil {
		return nil, err
	}

	cfg := &packages.Config{
		Mode:    mode,
		Tests:   true,
		Context: ctx,
		Dir:     res.Workspace.ModuleRoot,
	}

	slog.Info("loading packages", "root", res.Workspace.ModuleRoot, "pattern", res.Pattern)
	
	// Early check: if pattern is relative and path does not exist, fail clearly
	if strings.HasPrefix(res.Pattern, "./") {
		full := filepath.Join(res.Workspace.ModuleRoot, strings.TrimPrefix(res.Pattern, "./"))
		if strings.HasSuffix(full, "/...") {
			full = strings.TrimSuffix(full, "/...")
		} else if strings.HasSuffix(full, "...") {
			full = strings.TrimSuffix(full, "...")
		}
		
		if _, err := os.Stat(full); err != nil {
			return nil, fmt.Errorf("local path not found: %s", full)
		}
	}

	pkgs, err := packages.Load(cfg, res.Pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to load package: %v", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no package found for %s", pkgPath)
	}

	// Check for errors in loaded packages
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return nil, fmt.Errorf("package load error: %v", p.Errors[0])
		}
	}

	return pkgs, nil
}
