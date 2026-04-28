package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// DeleteByCategory purges all memory-domain records that match the exact category string
// by scanning the _idx:cat: secondary index. Rejects standards-domain categories.
func (s *MemoryStore) DeleteByCategory(ctx context.Context, category string) (int, error) {
	// Reject standards-domain categories upfront.
	if HarvestedCategories[category] {
		return 0, fmt.Errorf("category %q belongs to the standards domain; use standards tools to manage it", category)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Phase 1: Collect keys via category index, filtering by domain.
	var ids []string
	catPrefix := []byte(fmt.Sprintf("_idx:cat:%s:", strings.ToLower(category)))

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(catPrefix); it.ValidForPrefix(catPrefix); it.Next() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if err := it.Item().Value(func(kVal []byte) error {
				originalKey := string(kVal)
				item, err := txn.Get(kVal)
				if err != nil {
					return nil
				}
				return item.Value(func(v []byte) error {
					rec, err := migrateRecord(v)
					if err != nil {
						return nil
					}
					// Only collect memory-domain records.
					if rec.Domain == DomainMemories && rec.Category == category {
						ids = append(ids, originalKey)
					}
					return nil
				})
			}); err != nil {
				slog.Warn("Error scanning category index during delete", "category", category, "error", err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("category index scan failed: %w", err)
	}

	if len(ids) == 0 {
		return 0, nil
	}

	// Phase 2: Delete collected keys (already holding s.mu).
	for start := 0; start < len(ids); start += s.maxBatchSize {
		end := start + s.maxBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		if err := s.UpdateWithRetry(func(txn *badger.Txn) error {
			for _, k := range chunk {
				if err := s.deleteNoLockTxn(txn, k); err != nil && err != badger.ErrKeyNotFound {
					return err
				}
			}
			return nil
		}); err != nil {
			return 0, fmt.Errorf("failed deleting batched category hits: %w", err)
		}

		if s.search != nil {
			if sErr := s.search.DeleteBatch(chunk); sErr != nil {
				slog.Warn("Bleve index delete batch failed during category purge", "error", sErr)
			}
		}
	}

	return len(ids), nil
}

// DeleteBatch performs a single transaction badger deletion pass.
func (s *MemoryStore) DeleteBatch(ctx context.Context, keys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// It's possible for big requests to OOM or exceed max transaction sizes, limit to maxBatchSize
	for start := 0; start < len(keys); start += s.maxBatchSize {
		end := start + s.maxBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		chunk := keys[start:end]

		err := s.UpdateWithRetry(func(txn *badger.Txn) error {
			for _, k := range chunk {
				// We must resolve the payload locally to strip indices.
				if err := s.deleteNoLockTxn(txn, k); err != nil && err != badger.ErrKeyNotFound {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		if s.search != nil {
			if sErr := s.search.DeleteBatch(chunk); sErr != nil {
				slog.Warn("Bleve index delete batch failed during purge", "error", sErr)
			}
		}
	}
	return nil
}

// deleteNoLockTxn handles stripping indices within an existing transaction.
func (s *MemoryStore) deleteNoLockTxn(txn *badger.Txn, key string) error {
	item, err := txn.Get([]byte(key))
	if err != nil {
		return err
	}
	var rec *Record
	err = item.Value(func(val []byte) error {
		var mErr error
		rec, mErr = migrateRecord(val)
		return mErr
	})
	if err != nil {
		return err
	}
	s.deleteRecordIndices(txn, key, rec)
	return txn.Delete([]byte(key))
}

// DeleteByPath purges all records previously generated for a specific source path.
func (s *MemoryStore) DeleteByPath(ctx context.Context, sourcePath string) error {
	if s.search == nil {
		return fmt.Errorf("search engine required to bulk resolve paths")
	}

	// 1. We mapped sourcepath exactly as NewKeywordFieldMapping
	// Unfortunately, mapping exact matches via Search() means utilizing bleve.NewMatchQuery.
	// Since Search() abstracts bleve, we will just pass the exact string and filter post-search.
	hits, err := s.search.Search(ctx, sourcePath, []string{sourcePath}, 10000)
	if err != nil {
		return fmt.Errorf("failed resolving existing entries for path %s: %w", sourcePath, err)
	}

	var ids []string
	for _, h := range hits {
		rec, rErr := s.Get(ctx, h.ID)
		if rErr == nil && rec.SourcePath == sourcePath {
			ids = append(ids, h.ID)
		}
	}

	if len(ids) == 0 {
		return nil
	}

	return s.DeleteBatch(ctx, ids)
}

// processFile reads a file, checks its hash, executes DeleteByPath, and processes the AST into batched memory payloads.
func (s *MemoryStore) processFile(ctx context.Context, path string) ([]BatchEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 1. Hash verification.
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}
	currentHash := hex.EncodeToString(hasher.Sum(nil))
	hashKey := fmt.Sprintf("file_hash:%s", path)

	// Fetch existing
	s.mu.RLock()
	var priorHash string
	if err := s.db.View(func(txn *badger.Txn) error {
		itm, err := txn.Get([]byte(hashKey))
		if err == nil {
			if vErr := itm.Value(func(val []byte) error {
				priorHash = string(val)
				return nil
			}); vErr != nil {
				slog.Warn("Failed to fetch hash value", "path", path, "err", vErr)
			}
		}
		return nil
	}); err != nil {
		slog.Warn("Failed to query hash prior value", "path", path, "err", err)
	}
	s.mu.RUnlock()

	if priorHash == currentHash {
		// Log "No changes detected" logic.
		slog.Debug("No changes detected during ingest file check", "path", path)
		return nil, nil // No entries means nothing to process or update.
	}

	// 2. Clear old ghost records.
	if err := s.DeleteByPath(ctx, path); err != nil {
		slog.Warn("Failed deleting path ghosts during re-ingest", "path", path, "err", err)
	}

	// 3. Update hash lock.
	s.mu.Lock()
	if err := s.UpdateWithRetry(func(txn *badger.Txn) error {
		return txn.Set([]byte(hashKey), []byte(currentHash))
	}); err != nil {
		slog.Error("Failed to update hash lock", "path", path, "err", err)
	}
	s.mu.Unlock()

	// 4. Parse content using standard Go clipping tools.
	file.Seek(0, 0)
	rawBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	contentString := string(rawBytes)

	// Use format-specific clipping
	ext := strings.ToLower(filepath.Ext(path))
	var entries []BatchEntry

	switch ext {
	case ".md":
		entries = s.clipMarkdown(path, contentString, currentHash)
	case ".yaml", ".yml":
		entries = s.clipYaml(path, contentString, currentHash)
	default:
		// Basic ingest entire file as one record for JSON/XML/TXT.
		filename := filepath.Base(path)
		entries = []BatchEntry{
			{
				Title:      filename,
				Key:        fmt.Sprintf("file:%s", filename),
				Value:      contentString,
				Category:   "ingest",
				Tags:       []string{strings.TrimPrefix(ext, ".")},
				SourcePath: path,
				SourceHash: currentHash,
			},
		}
	}

	return entries, nil
}

// clipMarkdown structurally loops chunks using '#' identifiers while preserving headers.
func (s *MemoryStore) clipMarkdown(path string, src string, hash string) []BatchEntry {
	var results []BatchEntry
	filename := filepath.Base(path)

	r := regexp.MustCompile(`(?m)^#+\s+(.*)`)
	matches := r.FindAllStringSubmatchIndex(src, -1)

	if len(matches) == 0 {
		return append(results, BatchEntry{
			Title:      filename,
			Key:        fmt.Sprintf("md_chunk_0:%s", filename),
			Value:      src,
			Category:   "documentation",
			Tags:       []string{"markdown"},
			SourcePath: path,
			SourceHash: hash,
		})
	}

	type chunkDef struct{ title, content string }
	var chunks []chunkDef
	lastIndex := 0
	lastTitle := filename

	for i, match := range matches {
		start := match[0]
		if i == 0 && start > 0 {
			content := strings.TrimSpace(src[0:start])
			if content != "" {
				chunks = append(chunks, chunkDef{title: lastTitle, content: content})
			}
		} else if i > 0 {
			content := strings.TrimSpace(src[lastIndex:start])
			if content != "" {
				chunks = append(chunks, chunkDef{title: lastTitle, content: content})
			}
		}
		lastTitle = strings.TrimSpace(src[match[2]:match[3]])
		lastIndex = start // include header line inside standard text block
	}

	if lastIndex < len(src) {
		content := strings.TrimSpace(src[lastIndex:])
		if content != "" {
			chunks = append(chunks, chunkDef{title: lastTitle, content: content})
		}
	}

	for i, chunk := range chunks {
		results = append(results, BatchEntry{
			Title:      chunk.title,
			Key:        fmt.Sprintf("md_chunk_%d:%s", i, filename),
			Value:      chunk.content,
			Category:   "documentation",
			Tags:       []string{"markdown"},
			SourcePath: path,
			SourceHash: hash,
		})
	}
	return results
}

// clipYaml separates multi-document yaml boundaries cleanly
func (s *MemoryStore) clipYaml(path string, src string, hash string) []BatchEntry {
	var results []BatchEntry
	filename := filepath.Base(path)
	chunks := strings.Split(src, "\n---")

	for i, chunk := range chunks {
		content := strings.TrimSpace(chunk)
		if content == "" {
			continue
		}

		results = append(results, BatchEntry{
			Title:      filename,
			Key:        fmt.Sprintf("yaml_chunk_%d:%s", i, filename),
			Value:      content,
			Category:   "configuration",
			Tags:       []string{"yaml"},
			SourcePath: path,
			SourceHash: hash,
		})
	}
	return results
}

// ProcessPath handles file dispatching and directory walking concurrently via workers.
func (s *MemoryStore) ProcessPath(ctx context.Context, rootPath string) (int, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		return 0, err
	}

	// Single File Short-circuit
	if !info.IsDir() {
		entries, err := s.processFile(ctx, rootPath)
		if err != nil {
			return 0, err
		}
		if len(entries) > 0 {
			stored, errs, batchErr := s.SaveBatch(ctx, entries)
			if len(errs) > 0 {
				slog.Warn("ProcessPath single-file SaveBatch generated partial errors", "error_count", len(errs))
			}
			return stored, batchErr
		}
		return 0, nil
	}

	// Concurrent Directory Iteration
	pathsCh := make(chan string, 50)

	// Producer
	go func(c context.Context) {
		defer close(pathsCh)
		filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "vendor" || name == ".idea" {
					return filepath.SkipDir
				}
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".md" || ext == ".yaml" || ext == ".yml" || ext == ".json" || ext == ".txt" || ext == ".xml" {
				select {
				case pathsCh <- path:
				case <-c.Done():
					return c.Err()
				}
			}
			return nil
		})
	}(ctx)

	var wg sync.WaitGroup
	workers := runtime.NumCPU()
	if workers > 2 {
		workers = 2 // Cap workers for Bastion resource constraints
	}
	wg.Add(workers)

	recordCh := make(chan []BatchEntry, 100)

	for i := 0; i < workers; i++ {
		go func(c context.Context) {
			defer wg.Done()
			for p := range pathsCh {
				select {
				case <-c.Done():
					return
				default:
				}
				entries, pErr := s.processFile(c, p)
				if pErr != nil {
					slog.Warn("Failed parsing file", "path", p, "error", pErr)
					continue
				}
				if len(entries) > 0 {
					select {
					case recordCh <- entries:
					case <-c.Done():
						return
					}
				}
			}
		}(ctx)
	}

	go func(c context.Context) {
		wg.Wait()
		close(recordCh)
	}(ctx)

	totalStored := 0
	for batchedNodes := range recordCh {
		stored, errs, err := s.SaveBatch(ctx, batchedNodes)
		if len(errs) > 0 {
			slog.Warn("ProcessPath concurrent SaveBatch generated partial errors", "error_count", len(errs))
		}
		if err != nil {
			slog.Error("Failed saving batched nodes", "error", err)
		}
		totalStored += stored
		// Throttling for I/O pacing and compaction relief during bulk loads
		time.Sleep(50 * time.Millisecond)
	}

	return totalStored, nil
}
