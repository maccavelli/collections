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

// AnalyzeGenericsDirectory recursively scans a directory for Go files
// and identifies generic interface bloat patterns that can be
// simplified using Go 1.26 self-referential generics.
func (i *Inspector) AnalyzeGenericsDirectory(
	ctx context.Context, root string,
) ([]models.Gap, error) {
	absRoot, errPath := filepath.Abs(root)
	if errPath == nil {
		root = absRoot
	}
	aggKey := fmt.Sprintf("brainstorm:generic_agg:%x", sha256.Sum256([]byte(root)))

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
	for w := 0; w < numWorkers; w++ {
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

					fileGaps, err := i.processGenericFile(c, fset, path)
					if err != nil {
						slog.Error("AST parsing error for generic bloat detection", "file", path, "error", err)
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

func (i *Inspector) processGenericFile(ctx context.Context, fset *token.FileSet, path string) ([]models.Gap, error) {
	info, err := os.Stat(path)
	if err != nil {
		atomic.AddUint64(&i.metrics.Misses, 1)
		fileGaps, parseErr := i.analyzeGenericFile(ctx, fset, path)
		if parseErr != nil {
			return nil, fmt.Errorf("stat and generic parse failed: %w", parseErr)
		}
		return fileGaps, nil
	}

	const maxFileSize = 5 * 1024 * 1024 // 5MB
	if info.Size() > maxFileSize {
		slog.Warn("skipping oversized file for generic analysis", "file", path, "size", info.Size())
		return nil, nil
	}

	hash, err := i.getContentHash(path)
	if err != nil {
		slog.Warn("failed to compute content hash", "file", path, "err", err)
	}
	key := fmt.Sprintf("brainstorm:generic:%s:%s", hash, path)

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
	fileGaps, err := i.analyzeGenericFile(ctx, fset, path)
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

func (i *Inspector) analyzeGenericFile(
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
		case *ast.TypeSpec:
			if iface, ok := fn.Type.(*ast.InterfaceType); ok {
				gaps = append(gaps, i.checkInterfaceBloat(fset, fn.Name.Name, fn.TypeParams, iface, relPath)...)
			}
		}
		return true
	})

	return gaps, nil
}

// checkInterfaceBloat flags interfaces with multiple type parameters as a symptom
// of missing Go 1.26 self-referential generic optimization.
func (i *Inspector) checkInterfaceBloat(
	fset *token.FileSet, name string, tparams *ast.FieldList, iface *ast.InterfaceType, relPath string,
) []models.Gap {
	if tparams == nil || len(tparams.List) == 0 {
		return nil
	}

	paramCount := 0
	for _, field := range tparams.List {
		if len(field.Names) == 0 {
			// e.g. type Foo[any] (invalid syntax natively, but handle it cleanly)
			paramCount++
		} else {
			paramCount += len(field.Names)
		}
	}

	if paramCount < 2 {
		return nil
	}

	line := fset.Position(iface.Interface).Line
	return []models.Gap{{
		Area:        "GENERICS_BLOAT",
		Description: fmt.Sprintf("Interface '%s' at %s:%d defines %d type parameters. By utilizing Go 1.26 self-referential generics (e.g. `[T Type[T]]`), you can often lock the 'Self-Type' directly into the constraint and significantly reduce interface bloat. Check implementing structs to see if you can collapse these parameters into a recursive constraint.", name, relPath, line, paramCount),
		Severity:    "RECOMMENDED",
	}}
}
