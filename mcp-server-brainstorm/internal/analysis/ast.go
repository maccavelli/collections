// Package analysis provides static analysis tools for Go
// source code.
package analysis

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mcp-server-brainstorm/internal/models"
)

// Inspector performs static analysis on Go source files.
type Inspector struct {
	cache   map[string]cacheResult
	cacheMu sync.RWMutex
}

type cacheResult struct {
	gaps    []models.Gap
	modTime time.Time
}

// NewInspector creates a new Go code inspector with an in-memory cache.
func NewInspector() *Inspector {
	return &Inspector{
		cache: make(map[string]cacheResult),
	}
}

// AnalyzeDirectory recursively scans a directory for Go
// files and identifies code quality gaps using AST analysis.
// It processes files in parallel using a worker pool and
// utilizes a cache for improved performance.
func (i *Inspector) AnalyzeDirectory(
	ctx context.Context, root string,
) ([]models.Gap, error) {
	const numWorkers = 8
	fset := token.NewFileSet()
	paths := make(chan string)
	results := make(chan []models.Gap)

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start worker pool.
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case path, ok := <-paths:
					if !ok {
						return
					}

					// Check cache first.
					info, err := os.Stat(path)
					if err == nil {
						i.cacheMu.RLock()
						entry, exists := i.cache[path]
						i.cacheMu.RUnlock()
						if exists && entry.modTime.Equal(info.ModTime()) {
							select {
							case results <- entry.gaps:
								continue
							case <-ctx.Done():
								return
							}
						}
					}

					fileGaps, err := i.analyzeFile(ctx, fset, path)
					if err != nil {
						slog.Error("AST parsing error", "file", path, "error", err)
						continue
					}

					// Update cache.
					if info != nil {
						i.cacheMu.Lock()
						i.cache[path] = cacheResult{
							gaps:    fileGaps,
							modTime: info.ModTime(),
						}
						i.cacheMu.Unlock()
					}

					select {
					case results <- fileGaps:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// Start result collector.
	var allGaps []models.Gap
	doneCollecting := make(chan bool)
	go func() {
		for res := range results {
			allGaps = append(allGaps, res...)
		}
		doneCollecting <- true
	}()

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

	ast.Inspect(node, func(n ast.Node) bool {
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
		return true
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
	if idx := strings.Index(s, "_sk_"); idx >= 0 {
		return true
	}
	if idx := strings.Index(s, "-sk_"); idx >= 0 {
		return true
	}

	return false
}

