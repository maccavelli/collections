// Package analysis provides static analysis tools for Go
// source code.
package structural

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	"golang.org/x/tools/go/ast/inspector"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/buntdb"
	"mcp-server-go-refactor/internal/models"
)

// Metrics tracks cache performance.
type Metrics struct {
	Hits   uint64
	Misses uint64
}

// Inspector performs static analysis on Go source files.
type Inspector struct {
	db      *buntdb.DB
	metrics *Metrics
}

type cacheResult struct {
	Gaps    []models.Gap `json:"gaps"`
	ModTime time.Time    `json:"modTime"`
}

// NewInspector creates a new Go code inspector with BuntDB caching.
func NewInspector(db *buntdb.DB) *Inspector {
	return &Inspector{
		db:      db,
		metrics: &Metrics{},
	}
}

// GetMetrics returns a snapshot of cache performance.
func (i *Inspector) GetMetrics() (hits, misses uint64, entries int) {
	hits = atomic.LoadUint64(&i.metrics.Hits)
	misses = atomic.LoadUint64(&i.metrics.Misses)
	if i.db != nil {
		err := i.db.View(func(tx *buntdb.Tx) error {
			var lenErr error
			entries, lenErr = tx.Len()
			return lenErr
		})
		if err != nil {
			slog.Warn("failed to get cache metrics len", "error", err)
		}
	}
	return
}

func (i *Inspector) processFile(ctx context.Context, fset *token.FileSet, path string) ([]models.Gap, error) {
	info, err := os.Stat(path)
	if err != nil {
		// Stat failed — attempt parse anyway but skip caching.
		atomic.AddUint64(&i.metrics.Misses, 1)
		fileGaps, parseErr := i.analyzeFile(ctx, fset, path)
		if parseErr != nil {
			return nil, fmt.Errorf("stat and parse failed: %w", parseErr)
		}
		return fileGaps, nil
	}

	// Fix #4: Skip oversized files to prevent OOM.
	const maxFileSize = 5 * 1024 * 1024 // 5MB
	if info.Size() > maxFileSize {
		slog.Warn("skipping oversized file for AST analysis", "file", path, "size", info.Size())
		return nil, nil
	}

	// Fix #3: Compute hash once, reuse for both lookup and store.
	hash, err := i.getContentHash(path)
	if err != nil {
		slog.Warn("failed to compute content hash", "file", path, "err", err)
	}
	// Hierarchical Prefix Standardization: e.g. "brainstorm:ast:<hash>:<path>"
	key := fmt.Sprintf("brainstorm:ast:%s:%s", hash, path)

	var entry cacheResult
	found := false
	if i.db != nil {
		errView := i.db.View(func(tx *buntdb.Tx) error {
			val, txErr := tx.Get(key)
			if txErr == nil {
				// JSON powers BuntDB Spatial Indexing
				if json.Unmarshal([]byte(val), &entry) == nil {
					found = true
				}
			}
			return nil
		})
		if errView != nil {
			slog.Warn("failed to read from buntdb cache", "file", path, "err", errView)
		}
	}

	if found {
		atomic.AddUint64(&i.metrics.Hits, 1)
		return entry.Gaps, nil
	}

	atomic.AddUint64(&i.metrics.Misses, 1)
	fileGaps, err := i.analyzeFile(ctx, fset, path)
	if err != nil {
		return nil, err
	}

	// Update cache with 4h ephemeral TTL
	if i.db != nil {
		newEntry := cacheResult{
			Gaps:    fileGaps,
			ModTime: info.ModTime(),
		}
		data, errMarshal := json.Marshal(newEntry)
		if errMarshal != nil {
			slog.Warn("failed to marshal cache entry", "err", errMarshal)
		} else {
			errUpdate := i.db.Update(func(tx *buntdb.Tx) error {
				// Ephemeral Sandboxing auto-GC
				_, _, txErr := tx.Set(key, string(data), &buntdb.SetOptions{
					Expires: true,
					TTL:     4 * time.Hour,
				})
				return txErr
			})
			if errUpdate != nil {
				slog.Warn("failed to write to buntdb cache", "file", path, "err", errUpdate)
			}
		}
	}

	return fileGaps, nil
}

// AnalyzeDirectory recursively scans a directory for Go
// files and identifies code quality gaps using AST analysis.
// It processes files in parallel using a worker pool and
// utilizes a cache for improved performance.
func (i *Inspector) AnalyzeDirectory(
	ctx context.Context, root string,
) ([]models.Gap, error) {
	absRoot, errPath := filepath.Abs(root)
	if errPath == nil {
		root = absRoot
	}
	aggKey := fmt.Sprintf("brainstorm:ast_agg:%x", sha256.Sum256([]byte(root)))

	var cachedGaps []models.Gap
	foundAgg := false

	if i.db != nil {
		_ = i.db.View(func(tx *buntdb.Tx) error {
			val, err := tx.Get(aggKey)
			if err == nil {
				if json.Unmarshal([]byte(val), &cachedGaps) == nil {
					foundAgg = true
				}
			}
			return err
		})
	}

	if foundAgg {
		return cachedGaps, nil
	}

	const numWorkers = 8
	fset := token.NewFileSet()
	paths := make(chan string)
	results := make(chan []models.Gap)

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start worker pool.
	for range numWorkers {
		wg.Add(1)
		go func(c context.Context) {
			defer wg.Done()
			for {
				select {
				case <-c.Done():
					return
				case path, ok := <-paths:
					if !ok {
						return
					}

					// Resource-Aware Throttling: Check memory pressure before parsing
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					// If HeapAlloc > 400MB (out of 512MB limit), wait for GC
					if m.HeapAlloc > 400*1024*1024 {
						runtime.GC()
						time.Sleep(100 * time.Millisecond)
					}

					fileGaps, err := i.processFile(c, fset, path)
					if err != nil {
						slog.Error("AST parsing error", "file", path, "error", err)
						continue
					}

					if len(fileGaps) > 0 {
						select {
						case results <- fileGaps:
						case <-c.Done():
							return
						}
					}
				}
			}
		}(ctx)
	}

	// Start result collector.
	var allGaps []models.Gap
	doneCollecting := make(chan bool, 1)
	go func(c context.Context) {
		defer func() { doneCollecting <- true }()
		for {
			select {
			case <-c.Done():
				return
			case res, ok := <-results:
				if !ok {
					return
				}
				allGaps = append(allGaps, res...)
			}
		}
	}(ctx)

	// Feed workers.
	rootDepth := strings.Count(root, string(os.PathSeparator))
	const maxDiscoveryDepth = 5 // Standardized depth ceiling

	err := filepath.WalkDir(
		root,
		func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				depth := strings.Count(path, string(os.PathSeparator)) - rootDepth
				if depth > maxDiscoveryDepth {
					return filepath.SkipDir
				}
				switch d.Name() {
				case ".git", "vendor", "node_modules":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") ||
				strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			select {
			case paths <- path:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	)

	close(paths)
	wg.Wait()
	close(results)
	<-doneCollecting

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	if i.db != nil {
		data, errM := json.Marshal(allGaps)
		if errM == nil {
			_ = i.db.Update(func(tx *buntdb.Tx) error {
				_, _, errS := tx.Set(aggKey, string(data), &buntdb.SetOptions{Expires: true, TTL: 5 * time.Minute})
				return errS
			})
		}
	}

	return allGaps, nil
}

func (i *Inspector) analyzeFile(
	ctx context.Context, fset *token.FileSet, path string,
) ([]models.Gap, error) {
	// Respect cancellation before starting heavy parsing.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var gaps []models.Gap
	node, err := parser.ParseFile(
		fset, path, nil, parser.ParseComments,
	)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(".", path)
	if err != nil {
		relPath = path // Fallback to full path on error.
	}

	insp := inspector.New([]*ast.File{node})
	insp.Preorder([]ast.Node{
		(*ast.FuncDecl)(nil), (*ast.AssignStmt)(nil),
		(*ast.ExprStmt)(nil), (*ast.BasicLit)(nil),
	}, func(n ast.Node) {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			gaps = append(
				gaps,
				i.checkFuncSize(fset, fn, relPath)...,
			)
			gaps = append(
				gaps,
				i.checkContextParam(fn, relPath)...,
			)

		case *ast.AssignStmt:
			gaps = append(
				gaps,
				i.checkBlankErrorAssign(
					fset, fn, relPath,
				)...,
			)

		case *ast.ExprStmt:
			gaps = append(
				gaps,
				i.checkExplicitDiscard(
					fset, fn, relPath,
				)...,
			)
			gaps = append(
				gaps,
				i.checkPanic(fset, fn, relPath)...,
			)

		case *ast.BasicLit:
			gaps = append(
				gaps,
				i.checkSecrets(fset, fn, relPath)...,
			)
		}
	})

	return gaps, nil
}

// checkFuncSize detects functions exceeding 100 lines.
func (i *Inspector) checkFuncSize(
	fset *token.FileSet,
	fn *ast.FuncDecl,
	relPath string,
) []models.Gap {
	startLine := fset.Position(fn.Pos()).Line
	endLine := fset.Position(fn.End()).Line
	if endLine-startLine > 100 {
		return []models.Gap{{
			Area: "CODE_COMPLEXITY",
			Description: fmt.Sprintf(
				"Function %s in %s is too large"+
					" (%d lines).",
				fn.Name.Name, relPath,
				endLine-startLine,
			),
			Severity: "RECOMMENDED",
		}}
	}
	return nil
}

// checkContextParam flags strategic functions that lack
// a context.Context parameter.
func (i *Inspector) checkContextParam(
	fn *ast.FuncDecl, relPath string,
) []models.Gap {
	if !strings.HasPrefix(fn.Name.Name, "Analyze") &&
		!strings.HasPrefix(fn.Name.Name, "Handle") {
		return nil
	}
	// Skip handler factories that return ToolHandlerFunc.
	if fn.Type.Results != nil && len(fn.Type.Results.List) == 1 {
		if sel, ok := fn.Type.Results.List[0].Type.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "ToolHandlerFunc" {
				return nil
			}
		}
	}
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			t, ok := field.Type.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			pkg, ok := t.X.(*ast.Ident)
			if ok && pkg.Name == "context" &&
				t.Sel.Name == "Context" {
				return nil
			}
		}
	}
	return []models.Gap{{
		Area: "STABILITY",
		Description: fmt.Sprintf(
			"Strategic function %s in %s lacks"+
				" context.Context parameter.",
			fn.Name.Name, relPath,
		),
		Severity: "RECOMMENDED",
	}}
}

// checkBlankErrorAssign detects patterns where a function
// call result is explicitly discarded with a blank
// identifier (e.g., `_ = someFunc()`), which may suppress
// important errors.
func (i *Inspector) checkBlankErrorAssign(
	fset *token.FileSet,
	stmt *ast.AssignStmt,
	relPath string,
) []models.Gap {
	// Only check short assignments or plain assigns.
	if stmt.Tok != token.ASSIGN &&
		stmt.Tok != token.DEFINE {
		return nil
	}
	// Need at least one LHS and one RHS.
	if len(stmt.Lhs) == 0 || len(stmt.Rhs) == 0 {
		return nil
	}

	// check if 'err' is present in LHS to avoid false positives.
	hasErr := false
	for _, lhs := range stmt.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok && ident.Name == "err" {
			hasErr = true
			break
		}
	}
	if hasErr {
		return nil
	}

	// Check if any LHS is a blank identifier assigned
	// from a function call.
	for idx, lhs := range stmt.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || ident.Name != "_" {
			continue
		}
		// Corresponding RHS should be a call expression
		// or the single RHS is a multi-return call.
		if idx < len(stmt.Rhs) {
			if _, isCall := stmt.Rhs[idx].(*ast.CallExpr); isCall {
				line := fset.Position(stmt.Pos()).Line
				return []models.Gap{{
					Area: "STABILITY",
					Description: fmt.Sprintf(
						"Possible suppressed error at"+
							" %s:%d — blank identifier"+
							" discards function result.",
						relPath, line,
					),
					Severity: "RECOMMENDED",
				}}
			}
		} else if len(stmt.Rhs) == 1 {
			// Multi-return: _ , err = f() or _, _ = f()
			if _, isCall := stmt.Rhs[0].(*ast.CallExpr); isCall {
				line := fset.Position(stmt.Pos()).Line
				return []models.Gap{{
					Area: "STABILITY",
					Description: fmt.Sprintf(
						"Possible suppressed error at"+
							" %s:%d — blank identifier"+
							" discards function result.",
						relPath, line,
					),
					Severity: "RECOMMENDED",
				}}
			}
		}
	}
	return nil
}

// checkExplicitDiscard flags cases where a result is simply not assigned.
func (i *Inspector) checkExplicitDiscard(
	fset *token.FileSet,
	stmt *ast.ExprStmt,
	relPath string,
) []models.Gap {
	call, ok := stmt.X.(*ast.CallExpr)
	if !ok {
		return nil
	}
	// We only care about specific critical functions for now.
	// This can be expanded to check function signatures.
	funStr := ""
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		funStr = fmt.Sprintf("%s.%s", fn.X, fn.Sel)
	case *ast.Ident:
		funStr = fn.Name
	}

	criticals := map[string]bool{
		"os.Remove":    true,
		"os.Rename":    true,
		"os.WriteFile": true,
	}

	if criticals[funStr] {
		line := fset.Position(stmt.Pos()).Line
		return []models.Gap{{
			Area: "STABILITY",
			Description: fmt.Sprintf(
				"Explicit error discard at %s:%d —"+
					" %s should be checked or logged.",
				relPath, line, funStr,
			),
			Severity: "RECOMMENDED",
		}}
	}
	return nil
}

// checkPanic flags functions that use panic() for error handling.
func (i *Inspector) checkPanic(
	fset *token.FileSet,
	stmt *ast.ExprStmt,
	relPath string,
) []models.Gap {
	call, ok := stmt.X.(*ast.CallExpr)
	if !ok {
		return nil
	}
	if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "panic" {
		line := fset.Position(stmt.Pos()).Line
		return []models.Gap{{
			Area: "STABILITY",
			Description: fmt.Sprintf(
				"Use of panic() detected at %s:%d. Use explicit error "+
					"return or recovery instead.",
				relPath, line,
			),
			Severity: "RECOMMENDED",
		}}
	}
	return nil
}

// checkSecrets detects potential hardcoded secrets in string literals.
func (i *Inspector) checkSecrets(
	fset *token.FileSet,
	lit *ast.BasicLit,
	relPath string,
) []models.Gap {
	if lit.Kind != token.STRING {
		return nil
	}
	val := strings.Trim(lit.Value, "`\"")
	if len(val) < 16 {
		return nil
	}

	// Heuristic for high entropy or specific markers.
	if i.isPotentialSecret(val) {
		line := fset.Position(lit.Pos()).Line
		return []models.Gap{{
			Area: "SECURITY",
			Description: fmt.Sprintf(
				"Potential hardcoded secret detected at %s:%d.",
				relPath, line,
			),
			Severity: "CRITICAL",
		}}
	}
	return nil
}

func (i *Inspector) isPotentialSecret(s string) bool {
	// Refined markers with stricter boundaries to avoid false positives
	// like "risk_level" containing "sk_".
	s = strings.ToLower(s)

	// Check for common secret prefixes followed by characters.
	prefixes := []string{"key-", "api_", "secret_", "token_"}
	for _, p := range prefixes {
		if strings.Contains(s, p) {
			return true
		}
	}

	// "sk_" is usually a prefix for Stripe/other keys.
	// We check if it is either at the beginning or has a boundary before it.
	if strings.HasPrefix(s, "sk_") {
		return true
	}
	if found := strings.Contains(s, "_sk_"); found {
		return true
	}
	if found := strings.Contains(s, "-sk_"); found {
		return true
	}

	return false
}

func (i *Inspector) getContentHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
