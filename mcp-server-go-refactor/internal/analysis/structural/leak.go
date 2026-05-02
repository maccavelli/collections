package structural

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/buntdb"
	"mcp-server-go-refactor/internal/models"
)

// AnalyzeLeaksDirectory recursively scans a directory for Go files
// and identifies potential goroutine leaks using static heuristics.
func (i *Inspector) AnalyzeLeaksDirectory(
	ctx context.Context, root string,
) ([]models.Gap, error) {
	absRoot, errPath := filepath.Abs(root)
	if errPath == nil {
		root = absRoot
	}
	aggKey := fmt.Sprintf("brainstorm:leak_agg:%x", sha256.Sum256([]byte(root)))

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

					fileGaps, err := i.processLeakFile(c, fset, path)
					if err != nil {
						slog.Error("AST parsing error for leak detection", "file", path, "error", err)
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
	err := filepath.WalkDir(
		root,
		func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
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

func (i *Inspector) processLeakFile(ctx context.Context, fset *token.FileSet, path string) ([]models.Gap, error) {
	info, err := os.Stat(path)
	if err != nil {
		atomic.AddUint64(&i.metrics.Misses, 1)
		fileGaps, parseErr := i.analyzeLeakFile(ctx, fset, path)
		if parseErr != nil {
			return nil, fmt.Errorf("stat and leak parse failed: %w", parseErr)
		}
		return fileGaps, nil
	}

	const maxFileSize = 5 * 1024 * 1024 // 5MB
	if info.Size() > maxFileSize {
		slog.Warn("skipping oversized file for leak analysis", "file", path, "size", info.Size())
		return nil, nil
	}

	hash, err := i.getContentHash(path)
	if err != nil {
		slog.Warn("failed to compute content hash", "file", path, "err", err)
	}
	key := fmt.Sprintf("brainstorm:leak:%s:%s", hash, path)

	var entry cacheResult
	found := false
	if i.db != nil {
		errView := i.db.View(func(tx *buntdb.Tx) error {
			val, txErr := tx.Get(key)
			if txErr == nil {
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
	fileGaps, err := i.analyzeLeakFile(ctx, fset, path)
	if err != nil {
		return nil, err
	}

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

func (i *Inspector) analyzeLeakFile(
	ctx context.Context, fset *token.FileSet, path string,
) ([]models.Gap, error) {
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
		relPath = path
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.SelectStmt:
			gaps = append(gaps, i.checkUnguardedSelect(fset, fn, relPath)...)
		case *ast.GoStmt:
			gaps = append(gaps, i.checkNakedGoroutine(fset, fn, relPath)...)
		}
		return true
	})

	// Optional: add checkTerminalChannel logic here in the future
	// checkTerminalChannel(fset, node, relPath)

	return gaps, nil
}

// checkUnguardedSelect flags 'select' blocks that lack a 'default' case
// or a 'ctx.Done()' case while performing channel operations.
func (i *Inspector) checkUnguardedSelect(
	fset *token.FileSet, stmt *ast.SelectStmt, relPath string,
) []models.Gap {
	hasGuard := false
	hasChanOp := false

	for _, s := range stmt.Body.List {
		cc, ok := s.(*ast.CommClause)
		if !ok {
			continue
		}
		if cc.Comm == nil { // default case
			hasGuard = true
			break
		}

		switch cc.Comm.(type) {
		case *ast.AssignStmt, *ast.SendStmt, *ast.ExprStmt:
			hasChanOp = true
		}

		if i.isCtxDoneComm(cc.Comm) {
			hasGuard = true
			break
		}
	}

	if !hasGuard && hasChanOp {
		line := fset.Position(stmt.Pos()).Line
		return []models.Gap{{
			Area:        "GOROUTINE_LEAK",
			Description: fmt.Sprintf("Unguarded select at %s:%d. This channel operation lacks a default or ctx.Done() case. If the consumer fails, the producer will hang forever. Should we add a context-aware timeout here?", relPath, line),
			Severity:    "CRITICAL",
		}}
	}
	return nil
}

func (i *Inspector) isCtxDoneComm(stmt ast.Stmt) bool {
	var expr ast.Expr
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		expr = s.X
	case *ast.AssignStmt:
		if len(s.Rhs) > 0 {
			expr = s.Rhs[0]
		}
	}

	ue, ok := expr.(*ast.UnaryExpr)
	if !ok || ue.Op != token.ARROW { // Check for <-
		return false
	}

	call, ok := ue.X.(*ast.CallExpr)
	if !ok {
		return false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	return sel.Sel.Name == "Done"
}

// checkNakedGoroutine flags 'go' statements that do not propagate a context.Context object.
func (i *Inspector) checkNakedGoroutine(
	fset *token.FileSet, stmt *ast.GoStmt, relPath string,
) []models.Gap {
	passedCtx := false

	for _, arg := range stmt.Call.Args {
		if ident, ok := arg.(*ast.Ident); ok {
			lower := strings.ToLower(ident.Name)
			if strings.Contains(lower, "ctx") || lower == "context" {
				passedCtx = true
				break
			}
		}
	}

	if fl, ok := stmt.Call.Fun.(*ast.FuncLit); ok {
		if fl.Type.Params != nil {
			for _, p := range fl.Type.Params.List {
				if sel, ok := p.Type.(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "Context" {
						passedCtx = true
						break
					}
				}
			}
		}
	}

	if !passedCtx {
		line := fset.Position(stmt.Pos()).Line
		return []models.Gap{{
			Area:        "GOROUTINE_LEAK",
			Description: fmt.Sprintf("Goroutine launched at %s:%d without context propagation. Missing cancellation paths are the primary cause of durable block leaks in Go 1.26. Consider accepting a context parameter.", relPath, line),
			Severity:    "RECOMMENDED",
		}}
	}
	return nil
}
