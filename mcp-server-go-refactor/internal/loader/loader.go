package loader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go/ast"
	"go/parser"
	"go/token"

	"github.com/tidwall/buntdb"
	"golang.org/x/sync/singleflight"
	"golang.org/x/tools/go/packages"

	"mcp-server-go-refactor/internal/runner"
	"mcp-server-go-refactor/internal/workspace"
)

// DefaultMode provides the standard set of flags needed for most AST analysis.
const DefaultMode = packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedImports

// SyntaxMode provides a lightweight set of flags for metric analysis without deep type parsing.
const SyntaxMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax

// cacheTTL controls how long in-memory package entries remain valid.
const cacheTTL = 10 * time.Minute

// CacheMetrics tracks cache performance with atomic counters.
type CacheMetrics struct {
	Hits   uint64
	Misses uint64
}

// cacheEntry wraps a cached package set with an expiration timestamp.
type cacheEntry struct {
	pkgs    []*packages.Package
	expires time.Time
}

// packageCache holds recently loaded package results with TTL-based eviction.
var packageCache = struct {
	sync.RWMutex
	m       map[string]*cacheEntry
	metrics *CacheMetrics
}{
	m:       make(map[string]*cacheEntry),
	metrics: &CacheMetrics{},
}

var (
	discoveryGroup singleflight.Group
	discoveryCache = struct {
		sync.RWMutex
		m map[string]*cachedDiscovery
	}{m: make(map[string]*cachedDiscovery)}
)

type cachedDiscovery struct {
	result  *Result
	expires time.Time
}

// GetPackageCacheMetrics returns a snapshot of the in-memory package cache performance.
func GetPackageCacheMetrics() (hits, misses uint64, entries int) {
	hits = atomic.LoadUint64(&packageCache.metrics.Hits)
	misses = atomic.LoadUint64(&packageCache.metrics.Misses)
	packageCache.RLock()
	entries = len(packageCache.m)
	packageCache.RUnlock()
	return
}

// ResolveDir identifies the working directory and pattern to use for a given package path.
// Deprecated: use Discover() instead. This function lacks sentinel walk-up and module context.
func ResolveDir(pkgPath string) (dir string, pattern string) {
	slog.Warn("ResolveDir called — use loader.Discover() for sentinel-aware resolution", "pkg", pkgPath)
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
func Discover(ctx context.Context, pkgPath string) (*Result, error) {
	p, recursive := parsePath(pkgPath)
	abs, isLocal := resolveAbsPath(p)

	// Check singleflight TTL global cache
	discoveryCache.RLock()
	if entry, ok := discoveryCache.m[abs]; ok && time.Now().Before(entry.expires) {
		discoveryCache.RUnlock()
		return entry.result, nil
	}
	discoveryCache.RUnlock()

	v, err, _ := discoveryGroup.Do(abs, func() (any, error) {
		res, err := executeDiscovery(ctx, pkgPath, p, abs, isLocal, recursive)
		if err != nil {
			return nil, err
		}
		discoveryCache.Lock()
		discoveryCache.m[abs] = &cachedDiscovery{
			result:  res,
			expires: time.Now().Add(5 * time.Minute),
		}
		discoveryCache.Unlock()
		return res, nil
	})

	if err != nil {
		return nil, err
	}
	return v.(*Result), nil
}

func fastLocalResolve(modPath string) string {
	data, err := os.ReadFile(modPath)
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

func executeDiscovery(ctx context.Context, pkgPath, p, abs string, isLocal, recursive bool) (*Result, error) {
	// FAST-PATH: File-System Short Circuit avoids `go list` subprocess completely
	if isLocal {
		modPath := filepath.Join(abs, "go.mod")
		if _, err := os.Stat(modPath); err == nil {
			modName := fastLocalResolve(modPath)
			if modName != "" {
				rel := "."
				if recursive {
					rel = "./..."
				}
				wInfo := &workspace.Info{
					AbsPath:       abs,
					ModuleRoot:    abs,
					ModuleName:    modName,
					RelativePkg:   "./",
					IsModule:      true,
					DiscoveryType: "fast-local",
				}
				slog.Debug("fast-path workspace discovery", "root", abs)
				return &Result{
					Workspace: wInfo,
					Runner:    runner.New(wInfo.ModuleRoot),
					Pattern:   wInfo.ResolvePattern(rel),
				}, nil
			}
		}
	}

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

	return nil, fmt.Errorf("failed to discover workspace for %s: %w: %s", pkgPath, err, serr)
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
	for range 4 {
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
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
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

	cmd := exec.CommandContext(tctx, runner.DefaultGoBinary, listArgs...)
	if dir != "" {
		cmd.Dir = dir
	}

	var out, serr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &serr

	if err := cmd.Run(); err != nil {
		return nil, serr.String(), fmt.Errorf("go list failed: %w", err)
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
	// Check early if path isn't local
	if after, ok := strings.CutPrefix(res.Pattern, "./"); ok {
		full := filepath.Join(res.Workspace.ModuleRoot, after)
		if before, ok := strings.CutSuffix(full, "/..."); ok {
			full = before
		} else if before, ok := strings.CutSuffix(full, "..."); ok {
			full = before
		}
		if _, err := os.Stat(full); err != nil {
			return nil, fmt.Errorf("local path not found: %s", full)
		}
	}
	return LoadPackagesWithResult(ctx, res, mode, pkgPath)
}

// LoadPackagesWithResult bypasses the Discover subprocess overhead if the Result is already known.
func LoadPackagesWithResult(ctx context.Context, res *Result, mode packages.LoadMode, pkgPath string) ([]*packages.Package, error) {
	cacheKey := fmt.Sprintf("%s:%s:%d", res.Workspace.ModuleRoot, res.Pattern, mode)

	// Check for content-hash invalidation via go.mod/go.sum
	modHash := moduleContentHash(res.Workspace.ModuleRoot)
	fullKey := cacheKey + ":" + modHash

	packageCache.RLock()
	if entry, ok := packageCache.m[fullKey]; ok && time.Now().Before(entry.expires) {
		packageCache.RUnlock()
		atomic.AddUint64(&packageCache.metrics.Hits, 1)
		slog.Debug("cache hit for packages", "key", cacheKey)
		return entry.pkgs, nil
	}
	packageCache.RUnlock()

	atomic.AddUint64(&packageCache.metrics.Misses, 1)

	cfg := &packages.Config{
		Mode:    mode,
		Tests:   true,
		Context: ctx,
		Dir:     res.Workspace.ModuleRoot,
	}

	// Inject optimized parser if types are not needed
	if mode&packages.NeedTypesInfo == 0 {
		cfg.ParseFile = func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, src, parser.SkipObjectResolution|parser.ParseComments)
		}
	}

	slog.Info("loading packages (cache miss)", "root", res.Workspace.ModuleRoot, "pattern", res.Pattern)

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

	packageCache.Lock()
	packageCache.m[fullKey] = &cacheEntry{
		pkgs:    pkgs,
		expires: time.Now().Add(cacheTTL),
	}
	packageCache.Unlock()

	return pkgs, nil
}

// moduleContentHash returns a SHA-256 hash of go.mod + go.sum for invalidation.
func moduleContentHash(moduleRoot string) string {
	h := sha256.New()
	for _, name := range []string{"go.mod", "go.sum"} {
		f, err := os.Open(filepath.Join(moduleRoot, name))
		if err != nil {
			continue
		}
		if _, err := io.Copy(h, f); err != nil {
			slog.Warn("cache invalidation partial hash generation failed", "file", name, "err", err)
		}
		f.Close()
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// CachedEvaluate wraps an execution block with BuntDB persistence if the file/package hash has not changed.
func CachedEvaluate[T any](db *buntdb.DB, cachePrefix string, moduleRoot string, evaluate func() (T, error)) (T, error) {
	if db == nil {
		return evaluate()
	}
	modHash := moduleContentHash(moduleRoot)
	// Hierarchical Prefix Standardization: e.g. "go-refactor:metrics:complexity:<hash>"
	fullKey := fmt.Sprintf("%s:%s", cachePrefix, modHash)

	var result T

	err := db.View(func(tx *buntdb.Tx) error {
		val, err := tx.Get(fullKey)
		if err != nil {
			return err
		}
		// JSON powers BuntDB Spatial Indexing
		return json.Unmarshal([]byte(val), &result)
	})

	if err == nil {
		slog.Debug("BuntDB cache hit", "key", fullKey)
		return result, nil
	}

	result, err = evaluate()
	if err != nil {
		return result, err
	}

	if data, mErr := json.Marshal(result); mErr == nil {
		if errUpdate := db.Update(func(tx *buntdb.Tx) error {
			// Ephemeral Sandboxing auto-GC: Clean cache after 4 hours
			_, _, err := tx.Set(fullKey, string(data), &buntdb.SetOptions{Expires: true, TTL: 4 * time.Hour})
			return err
		}); errUpdate != nil {
			slog.Warn("failed to update BuntDB cache", "err", errUpdate)
		}
	}

	return result, nil
}
