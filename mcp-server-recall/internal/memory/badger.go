package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/sahilm/fuzzy"
)

// MemoryStore manages the BadgerDB persistent storage for memories.
type MemoryStore struct {
	db     *badger.DB
	mu     sync.RWMutex
	stopGC chan struct{}
}

// NewMemoryStore initializes a new BadgerDB at the specified directory.
func NewMemoryStore(dbPath string) (*MemoryStore, error) {
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	opts := badger.DefaultOptions(dbPath).
		WithLogger(nil).
		WithSyncWrites(true).
		WithValueThreshold(1024).         // Keep small JSON records in the LSM tree
		WithIndexCacheSize(128 << 20).    // 128MB cache for keys and bloom filters
		WithBlockCacheSize(256 << 20)     // 256MB cache for data blocks

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}

	s := &MemoryStore{
		db:     db,
		stopGC: make(chan struct{}),
	}
	
	// Start background maintenance
	go s.runGC()

	slog.Info("MemoryStore initialized with maintenance", "path", dbPath)
	return s, nil
}

// runGC executes BadgerDB value log garbage collection periodically.
func (s *MemoryStore) runGC() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Run GC and ignore ErrNoRewrite
			for {
				err := s.db.RunValueLogGC(0.5)
				if err != nil {
					if err != badger.ErrNoRewrite {
						slog.Debug("Badger GC completed", "status", "idle")
					}
					break
				}
				slog.Info("Badger GC reclaimed space", "status", "active")
			}
		case <-s.stopGC:
			return
		}
	}
}

// Save stores or updates a memory Record in the database.
func (s *MemoryStore) Save(ctx context.Context, key, content string, category string, tags []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var oldRec *Record
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err == nil {
			_ = item.Value(func(val []byte) error {
				if old, err := migrateRecord(val); err == nil {
					oldRec = old
				}
				return nil
			})
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("lookup failure: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		// 1. Clear old indexes
		if oldRec != nil {
			oldTimeIdx := fmt.Sprintf("_idx:t:%x:%s", oldRec.UpdatedAt.UnixNano(), key)
			_ = txn.Delete([]byte(oldTimeIdx))

			// Clear old tag indices
			for _, t := range oldRec.Tags {
				oldTagIdx := fmt.Sprintf("_idx:tag:%s:%s", strings.ToLower(t), key)
				_ = txn.Delete([]byte(oldTagIdx))
			}

			// Clear old category index
			if oldRec.Category != "" {
				oldCatIdx := fmt.Sprintf("_idx:cat:%s:%s", strings.ToLower(oldRec.Category), key)
				_ = txn.Delete([]byte(oldCatIdx))
			}
		}

		// 2. Save actual record content
		rec := &Record{
			Content:   content,
			Category:  category,
			Tags:      tags,
			UpdatedAt: time.Now(),
		}
		if oldRec != nil {
			rec.CreatedAt = oldRec.CreatedAt
		} else {
			rec.CreatedAt = rec.UpdatedAt
		}

		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}

		if err := txn.Set([]byte(key), data); err != nil {
			return err
		}

		// 3. Save new time-based index (descending sort friendly)
		newTimeIdx := fmt.Sprintf("_idx:t:%x:%s", rec.UpdatedAt.UnixNano(), key)
		if err := txn.Set([]byte(newTimeIdx), []byte(key)); err != nil {
			return err
		}

		// 4. Save new category index
		if rec.Category != "" {
			newCatIdx := fmt.Sprintf("_idx:cat:%s:%s", strings.ToLower(rec.Category), key)
			if err := txn.Set([]byte(newCatIdx), []byte(key)); err != nil {
				return err
			}
		}

		// 5. Save new tag indices
		for _, t := range rec.Tags {
			newTagIdx := fmt.Sprintf("_idx:tag:%s:%s", strings.ToLower(t), key)
			if err := txn.Set([]byte(newTagIdx), []byte(key)); err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		slog.Debug("Memory saved and indexed", "key", key, "category", category, "tag_count", len(tags))
	}
	return err
}

// Get retrieves a Record from the database by key with auto-migration.
func (s *MemoryStore) Get(ctx context.Context, key string) (*Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var rec *Record
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			var mErr error
			rec, mErr = migrateRecord(val)
			return mErr
		})
	})

	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, fmt.Errorf("memory not found: %s", key)
		}
		return nil, err
	}
	return rec, nil
}

// Search matches keys, content, and tags with fuzzy relevance ranking and limits.
func (s *MemoryStore) Search(ctx context.Context, query string, tagFilter string, limit int) ([]*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var candidates []*SearchResult
	tagFilter = strings.ToLower(tagFilter)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		// Case A: Tag-based search (O(K) where K is records with that tag)
		if tagFilter != "" {
			prefix := []byte(fmt.Sprintf("_idx:tag:%s:", tagFilter))
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				// The value of the tag index is the original key
				_ = it.Item().Value(func(kVal []byte) error {
					originalKey := string(kVal)
					item, err := txn.Get(kVal)
					if err != nil {
						return nil
					}
					return item.Value(func(v []byte) error {
						if rec, err := migrateRecord(v); err == nil {
							candidates = append(candidates, &SearchResult{Key: originalKey, Record: rec})
						}
						return nil
					})
				})
			}
			return nil
		}

		// Case B: General search (Linear Scan, but avoiding non-data keys)
		for it.Rewind(); it.Valid(); it.Next() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			item := it.Item()
			k := string(item.Key())
			// Skip all internal indices
			if strings.HasPrefix(k, "_idx:") {
				continue
			}

			_ = item.Value(func(v []byte) error {
				rec, err := migrateRecord(v)
				if err != nil {
					return nil
				}
				candidates = append(candidates, &SearchResult{Key: k, Record: rec})
				return nil
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// If no query, return chronological/original order limited
	if query == "" {
		if limit > 0 && len(candidates) > limit {
			return candidates[:limit], nil
		}
		return candidates, nil
	}

	// Perform fuzzy matching and scoring
	results := s.rankCandidates(query, candidates)

	if limit > 0 && len(results) > limit {
		return results[:limit], nil
	}
	return results, nil
}

// rankCandidates applies fuzzy scoring across Key, Content, and Tags with parallelism.
func (s *MemoryStore) rankCandidates(query string, candidates []*SearchResult) []*SearchResult {
	if len(candidates) == 0 {
		return candidates
	}

	var wg sync.WaitGroup
	// Concurrency: Use worker pool based on CPU count or chunking
	workerCount := 4
	if len(candidates) < workerCount {
		workerCount = len(candidates)
	}
	
	chunkSize := (len(candidates) + workerCount - 1) / workerCount
	for i := 0; i < workerCount; i++ {
		start := i * chunkSize
		if start >= len(candidates) {
			break
		}
		end := start + chunkSize
		if end > len(candidates) {
			end = len(candidates)
		}

		wg.Add(1)
		go func(subset []*SearchResult) {
			defer wg.Done()
			for _, c := range subset {
				if c.Record == nil { continue }
				// 1. Score the Key
				keyMatches := fuzzy.Find(query, []string{c.Key})
				if len(keyMatches) > 0 {
					c.Score = keyMatches[0].Score
				}

				// 2. Score Content (if better)
				contentMatches := fuzzy.Find(query, []string{c.Record.Content})
				if len(contentMatches) > 0 && contentMatches[0].Score > c.Score {
					c.Score = contentMatches[0].Score
				}

				// 3. Score Category
				if c.Record.Category != "" {
					catMatches := fuzzy.Find(query, []string{c.Record.Category})
					if len(catMatches) > 0 && catMatches[0].Score > c.Score {
						c.Score = catMatches[0].Score
					}
				}

				// 4. Score Tags (if better)
				for _, t := range c.Record.Tags {
					tagMatches := fuzzy.Find(query, []string{t})
					if len(tagMatches) > 0 && tagMatches[0].Score > c.Score {
						c.Score = tagMatches[0].Score
					}
				}
			}
		}(candidates[start:end])
	}
	wg.Wait()

	// Filter out zero scores and sort
	var ranked []*SearchResult
	for _, c := range candidates {
		if c.Score > 0 {
			ranked = append(ranked, c)
		}
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score // Descending
	})

	return ranked
}

// GetRecent retrieves the last N memories sorted by UpdatedAt descending.
func (s *MemoryStore) GetRecent(ctx context.Context, count int) ([]*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*SearchResult
	if count <= 0 {
		return results, nil
	}

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Reverse = true // Latest first
		
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("_idx:t:")
		seekKey := append([]byte(nil), prefix...)
		seekKey = append(seekKey, 0xff, 0xff, 0xff)

		for it.Seek(seekKey); it.ValidForPrefix(prefix) && len(results) < count; it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				originalKey := val
				item, err := txn.Get(originalKey)
				if err != nil {
					return nil
				}

				return item.Value(func(v []byte) error {
					if rec, err := migrateRecord(v); err == nil {
						results = append(results, &SearchResult{
							Key:    string(originalKey),
							Record: rec,
						})
					}
					return nil
				})
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	return results, err
}

// ListKeys retrieves all available keys for knowledge discovery.
func (s *MemoryStore) ListKeys(ctx context.Context) ([]*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*SearchResult
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			k := string(it.Item().Key())
			if strings.HasPrefix(k, "_idx:") {
				continue
			}
			
			_ = it.Item().Value(func(v []byte) error {
				if rec, err := migrateRecord(v); err == nil {
					results = append(results, &SearchResult{Key: k, Record: rec})
				}
				return nil
			})
		}
		return nil
	})
	return results, err
}

// Clear removes all stored memories by dropping and recreating or clearing.
func (s *MemoryStore) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.DropAll()
}

// GetStats returns usage statistics about the memory store.
func (s *MemoryStore) GetStats() (int, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if !strings.HasPrefix(string(it.Item().Key()), "_idx:") {
				count++
			}
		}
		return nil
	})
	
	// We return entry count and size estimates
	lsm, vlog := s.db.Size()
	return count, lsm + vlog, err
}

// Delete removes a specific memory and its secondary index.
func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.deleteNoLock(key)
}

func (s *MemoryStore) deleteNoLock(key string) error {
	var rec *Record
	_ = s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err == nil {
			_ = item.Value(func(val []byte) error {
				rec, _ = migrateRecord(val)
				return nil
			})
		}
		return nil
	})

	return s.db.Update(func(txn *badger.Txn) error {
		if rec != nil {
			// Clear time index
			idxKey := fmt.Sprintf("_idx:t:%x:%s", rec.UpdatedAt.UnixNano(), key)
			_ = txn.Delete([]byte(idxKey))

			// Clear all tag indices
			for _, t := range rec.Tags {
				tagIdx := fmt.Sprintf("_idx:tag:%s:%s", strings.ToLower(t), key)
				_ = txn.Delete([]byte(tagIdx))
			}

			// Clear category index
			if rec.Category != "" {
				catIdx := fmt.Sprintf("_idx:cat:%s:%s", strings.ToLower(rec.Category), key)
				_ = txn.Delete([]byte(catIdx))
			}
		}
		return txn.Delete([]byte(key))
	})
}

// Consolidate identifies and merges redundant memories based on content similarity.
func (s *MemoryStore) Consolidate(ctx context.Context, threshold float64, dryRun bool) (int, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if threshold <= 0 {
		threshold = 0.8
	}

	// 1. Fetch all records for analysis
	var all []*SearchResult
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			k := it.Item().KeyCopy(nil)
			if strings.HasPrefix(string(k), "_idx:") {
				continue
			}
			_ = it.Item().Value(func(v []byte) error {
				if rec, err := migrateRecord(v); err == nil {
					all = append(all, &SearchResult{Key: string(k), Record: rec})
				}
				return nil
			})
		}
		return nil
	})
	if err != nil {
		return 0, nil, err
	}

	if len(all) < 2 {
		return 0, nil, nil
	}

	// 2. Perform pairwise similarity check and clustering (Find Connected Components)
	parent := make(map[string]string)
	for _, res := range all {
		parent[res.Key] = res.Key
	}

	var find func(string) string
	find = func(i string) string {
		if parent[i] == i {
			return i
		}
		parent[i] = find(parent[i])
		return parent[i]
	}

	union := func(i, j string) {
		rootI := find(i)
		rootJ := find(j)
		if rootI != rootJ {
			parent[rootI] = rootJ
		}
	}

	// Parallel Pairwise Comparison
	var wg sync.WaitGroup
	// Actually, let's use a simpler parallel approach for pairs:
	// Use a worker group to score chunks of the outer loop.
	
	workerCount := 4
	chunkSize := (len(all) + workerCount - 1) / workerCount
	var mergeMu sync.Mutex

	for w := 0; w < workerCount; w++ {
		start := w * chunkSize
		if start >= len(all) { break }
		end := start + chunkSize
		if end > len(all) { end = len(all) }

		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			for i := s; i < e; i++ {
				for j := i + 1; j < len(all); j++ {
					if computeJaccard(all[i].Record.Content, all[j].Record.Content) >= threshold {
						mergeMu.Lock()
						union(all[i].Key, all[j].Key)
						mergeMu.Unlock()
					}
				}
			}
		}(start, end)
	}
	wg.Wait()

	// 3. Group by root
	clusters := make(map[string][]*SearchResult)
	for _, res := range all {
		root := find(res.Key)
		clusters[root] = append(clusters[root], res)
	}

	// 4. Process clusters with > 1 members
	var mergedKeys []string
	mergeCount := 0
	for _, members := range clusters {
		if len(members) <= 1 {
			continue
		}

		// Identify primary (longest content as proxy for 'most descriptive')
		primary := members[0]
		for _, m := range members[1:] {
			if len(m.Record.Content) > len(primary.Record.Content) {
				primary = m
			}
		}

		// Merge tags
		tagSet := make(map[string]struct{})
		for _, m := range members {
			for _, t := range m.Record.Tags {
				tagSet[strings.ToLower(t)] = struct{}{}
			}
		}
		var mergedTags []string
		for t := range tagSet {
			mergedTags = append(mergedTags, t)
		}

		if dryRun {
			mergeCount++
			for _, m := range members {
				if m.Key != primary.Key {
					mergedKeys = append(mergedKeys, m.Key)
				}
			}
			continue
		}

		// Update database: Atomic operation for this cluster
		// Save unique tags to primary
		primary.Record.Tags = mergedTags
		data, _ := json.Marshal(primary.Record)
		
		err := s.db.Update(func(txn *badger.Txn) error {
			// Update primary
			if err := txn.Set([]byte(primary.Key), data); err != nil {
				return err
			}
			// Delete redundant ones (indices included)
			for _, m := range members {
				if m.Key == primary.Key {
					continue
				}
				mergedKeys = append(mergedKeys, m.Key)
				// We need the record to delete indices, but we already have it in 'm'
				idxKey := fmt.Sprintf("_idx:t:%x:%s", m.Record.UpdatedAt.UnixNano(), m.Key)
				_ = txn.Delete([]byte(idxKey))
				for _, t := range m.Record.Tags {
					tagIdx := fmt.Sprintf("_idx:tag:%s:%s", strings.ToLower(t), m.Key)
					_ = txn.Delete([]byte(tagIdx))
				}
				_ = txn.Delete([]byte(m.Key))
			}
			return nil
		})

		if err == nil {
			mergeCount++
			slog.Info("Consolidated memory cluster", "primary", primary.Key, "merged_redundants", len(members)-1)
		}
	}

	return mergeCount, mergedKeys, nil
}

// ListCategories retrieves a unique list of all memory categories with counts.
func (s *MemoryStore) ListCategories(ctx context.Context) (map[string]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	categories := make(map[string]int)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("_idx:cat:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); {
			item := it.Item()
			key := string(item.Key())
			// Key format: _idx:cat:<category>:<record_key>
			parts := strings.Split(key, ":")
			if len(parts) >= 3 {
				cat := parts[2]
				categories[cat]++
				
				// Optimization: Skip all records in THIS category to find the NEXT unique category
				// BUT we want counts, so we can't skip if we want exact counts.
				// If we only wanted names, we'd seek(append(prefixWithCat, 0xff))
				// Let's stick to full scan for accuracy in small-medium DBs.
				it.Next()
			} else {
				it.Next()
			}
		}
		return nil
	})
	return categories, err
}

func computeJaccard(a, b string) float64 {
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			if strings.ContainsRune("!.,:;?()[]{}", r) {
				return -1
			}
			return r
		}, strings.ToLower(s))
	}

	setA := make(map[string]struct{})
	for _, w := range strings.Fields(clean(a)) {
		if len(w) > 2 {
			setA[w] = struct{}{}
		}
	}
	setB := make(map[string]struct{})
	for _, w := range strings.Fields(clean(b)) {
		if len(w) > 2 {
			setB[w] = struct{}{}
		}
	}
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}
	inter := 0
	for w := range setA {
		if _, ok := setB[w]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	return float64(inter) / float64(union)
}

// Close safely shuts down the database and maintenance routines.
func (s *MemoryStore) Close() error {
	close(s.stopGC)
	return s.db.Close()
}
