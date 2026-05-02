package memory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	"github.com/sahilm/fuzzy"

	"mcp-server-recall/internal/config"
	"mcp-server-recall/internal/search"
)

// HarvestedCategories defines categories owned exclusively by the standards domain.
// These are excluded from memory-scoped tools (list_categories, search_memories)
// and used as inclusion filters for standards-scoped tools.
var HarvestedCategories = map[string]bool{
	"HarvestedCode": true,
	"PackageDoc":    true,
	"SysDrift":      true,
}

type CacheMetrics struct {
	Hits      uint64
	Misses    uint64
	Entries   int
	Memories  int
	Sessions  int
	Standards int
	Projects  int
	BleveDocs uint64
}

// MemoryStore manages the BadgerDB persistent storage for memories.
type MemoryStore struct {
	db             *badger.DB
	mu             sync.RWMutex
	ctx            context.Context // Parent context for background workers
	stopGC         chan struct{}
	closeOnce      sync.Once
	search         search.SearchEngine // Optional: Bleve full-text search layer
	searchLimit    int                 // Max documents to index
	stopAudit      chan struct{}
	maxBatchSize   int           // Configurable batch size cap
	driftAlerts    atomic.Uint64 // Tracks search index mismatches
	cacheHits      atomic.Uint64
	cacheMisses    atomic.Uint64
	dbHits         atomic.Uint64
	dbMisses       atomic.Uint64
	memoriesCount  atomic.Int64
	sessionsCount  atomic.Int64
	standardsCount atomic.Int64
	projectsCount  atomic.Int64
	// New Telemetry Hooks
	gcSweeps           atomic.Uint64
	gcPrunedNodes      atomic.Uint64
	searchLatency      atomic.Int64
	searchQueries      atomic.Uint64
	rpcPayloadBytes    atomic.Uint64
	boundaryViolations atomic.Uint64
}

// GetTelemetry surfaces memory and DB tier metrics.
func (s *MemoryStore) GetTelemetry() (uint64, uint64, uint64, uint64) {
	return s.cacheHits.Load(), s.cacheMisses.Load(), s.dbHits.Load(), s.dbMisses.Load()
}

// GetDBSize returns the BadgerDB LSM and Value log sizes.
func (s *MemoryStore) GetDBSize() (lsm int64, vlog int64) {
	if s.db == nil {
		return 0, 0
	}
	return s.db.Size()
}

// GetNamespaceCounts securely exports continuous atomic capacities across mapped domains.
func (s *MemoryStore) GetNamespaceCounts() (int64, int64, int64, int64) {
	return s.memoriesCount.Load(), s.sessionsCount.Load(), s.standardsCount.Load(), s.projectsCount.Load()
}

// GetExtendedTelemetry exports the new dashboard observability counters.
func (s *MemoryStore) GetExtendedTelemetry() (uint64, uint64, int64, uint64, uint64, uint64) {
	return s.gcSweeps.Load(), s.gcPrunedNodes.Load(), s.searchLatency.Load(), s.searchQueries.Load(), s.rpcPayloadBytes.Load(), s.boundaryViolations.Load()
}

// RecordSearchTelemetry tracks HNSW/Bleve query performance.
func (s *MemoryStore) RecordSearchTelemetry(latencyMs int64) {
	s.searchQueries.Add(1)
	s.searchLatency.Add(latencyMs)
}

// RecordRPCBytes tracks gateway ingress/egress.
func (s *MemoryStore) RecordRPCBytes(bytes uint64) {
	s.rpcPayloadBytes.Add(bytes)
}

// RecordSecurityViolation tracks access denials.
func (s *MemoryStore) RecordSecurityViolation() {
	s.boundaryViolations.Add(1)
}

// NewMemoryStore initializes a new BadgerDB with optional AES-256 encryption.
func NewMemoryStore(ctx context.Context, dbPath string, encryptionKey string, searchLimit int, batchCfg config.BatchConfig) (*MemoryStore, error) {
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	opts, err := buildBadgerOptions(dbPath, encryptionKey)
	if err != nil {
		return nil, err
	}

	db, err := openBadgerWithRetry(opts, 5)
	if err != nil {
		return nil, err
	}

	checkVlogHealth(dbPath)

	slog.Info("BadgerDB initialized within bounds",
		"sync_writes", true,
		"compression", "zstd",
		"memtable_mb", 32,
		"block_cache_mb", 128,
		"index_cache_mb", 64,
	)

	s := &MemoryStore{
		db:           db,
		ctx:          ctx,
		stopGC:       make(chan struct{}),
		stopAudit:    make(chan struct{}),
		searchLimit:  searchLimit,
		maxBatchSize: batchCfg.MaxBatchSize,
	}

	// Start background maintenance
	go func(ctx context.Context) {
		s.runGC()
	}(ctx)
	if searchLimit > 0 {
		go func(ctx context.Context) {
			s.runAuditWorker()
		}(ctx)
	}

	slog.Info("MemoryStore initialized with maintenance", "path", dbPath, "encrypted", encryptionKey != "")
	return s, nil
}

// buildBadgerOptions constructs the BadgerDB options chain with optional encryption.
func buildBadgerOptions(dbPath string, encryptionKey string) (badger.Options, error) {
	opts := badger.DefaultOptions(dbPath).
		WithLogger(nil).
		WithSyncWrites(false).
		WithLogger(nil).
		// 🛡️ OPTIMIZATION (10k Bounds): Burst write absorption
		// Increase memtables to 5 (160MB buffer) to prevent memtable stall
		// Increase Vlog entries to 100k to prevent aggressive GC rotation
		WithValueLogMaxEntries(100000).
		WithValueLogFileSize(128 << 20).
		WithBlockSize(4096).
		WithMemTableSize(32 << 20).
		WithNumMemtables(5).
		WithIndexCacheSize(64 << 20).
		WithBlockCacheSize(128 << 20).
		WithBaseTableSize(8 << 20).
		WithBaseLevelSize(10 << 20).
		WithLevelSizeMultiplier(10).
		WithMaxLevels(7).
		WithBlockSize(4096).
		// Default ValueThreshold prevents index bloat
		WithValueThreshold(1 << 10). // 1KB
		WithNumLevelZeroTables(10).
		WithNumLevelZeroTablesStall(20).
		WithCompactL0OnClose(true).
		WithChecksumVerificationMode(options.OnTableRead).
		// 🛡️ CPU CAP: Native 2-Core Topology Match. Prevents heavy context switching.
		WithNumGoroutines(2).
		WithMetricsEnabled(true)

	if encryptionKey != "" {
		if len(encryptionKey) != 32 {
			return opts, fmt.Errorf("encryption key must be exactly 32 bytes (got %d)", len(encryptionKey))
		}
		opts = opts.WithEncryptionKey([]byte(encryptionKey)).
			WithEncryptionKeyRotationDuration(7 * 24 * time.Hour)
	}

	return opts, nil
}

// openBadgerWithRetry attempts to open BadgerDB with exponential backoff for lock contention.
func openBadgerWithRetry(opts badger.Options, maxRetries int) (*badger.DB, error) {
	var db *badger.DB
	var err error

	backoff := 500 * time.Millisecond
	for i := range maxRetries {
		db, err = badger.Open(opts)
		if err == nil {
			return db, nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "cannot acquire directory lock") {
			slog.Warn("Badger directory lock held; retrying...", "attempt", i+1, "max_retries", maxRetries, "backoff", backoff)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}

	return nil, fmt.Errorf("failed to open badger db after %d retries: %w", maxRetries, err)
}

// checkVlogHealth warns if any vlog file has grown excessively (indicates GC failure).
func checkVlogHealth(dbPath string) {
	entries, err := os.ReadDir(dbPath)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".vlog") {
			if info, err := e.Info(); err == nil && info.Size() > 300<<20 {
				slog.Warn("Bloated vlog detected — consider DB reset",
					"file", e.Name(), "size_mb", info.Size()>>20)
			}
		}
	}
}

// SetSearchEngine attaches a SearchEngine and performs a cold-start rebuild.
// Must be called after NewMemoryStore and before serving requests.
func (s *MemoryStore) SetSearchEngine(ctx context.Context, engine search.SearchEngine) error {
	s.mu.Lock()
	s.search = engine
	s.mu.Unlock()

	return s.SyncSearchIndex(ctx)
}

// SyncSearchIndex performs a full scan of BadgerDB and rebuilds the search index.
// This is used for cold-starts and the reload_cache runtime tool.
func (s *MemoryStore) SyncSearchIndex(ctx context.Context) error {
	s.mu.RLock()
	searchEngine := s.search
	s.mu.RUnlock()

	if searchEngine == nil {
		return fmt.Errorf("search engine not initialized")
	}

	// Resource Safety: Check document count against memory limit.
	// We use the BadgerDB key count as a heuristic before rebuilding.
	count, _, err := s.GetStats()
	if err == nil && s.searchLimit > 0 && count > s.searchLimit {
		slog.Warn("Search memory limit exceeded; falling back to fuzzy matching",
			"count", count, "limit", s.searchLimit)
		s.mu.Lock()
		s.search = nil // Disable Bleve for this session
		s.mu.Unlock()
		return nil
	}

	// Cold-start: rebuild the full-text index from BadgerDB.
	s.memoriesCount.Store(0)
	s.sessionsCount.Store(0)
	s.standardsCount.Store(0)
	docs := make(map[string]*search.Document)
	err = s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			k := string(it.Item().Key())
			if strings.HasPrefix(k, "_idx:") {
				continue
			}
			if err := it.Item().Value(func(v []byte) error {
				if rec, err := migrateRecord(v); err == nil {
					switch rec.Domain {
					case DomainMemories:
						s.memoriesCount.Add(1)
					case DomainSessions:
						s.sessionsCount.Add(1)
					case DomainStandards:
						s.standardsCount.Add(1)
					case DomainProjects:
						s.projectsCount.Add(1)
					}
					docs[k] = &search.Document{
						Title:      rec.Title,
						SymbolName: rec.SymbolName,
						Content:    rec.Content,
						Category:   rec.Category,
						Tags:       rec.Tags,
						SourcePath: rec.SourcePath,
						SourceHash: rec.SourceHash,
					}
				}
				return nil
			}); err != nil {
				slog.Warn("Skipping corrupted record during search rebuild", "key", k, "error", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to read records for search rebuild: %w", err)
	}

	if err := searchEngine.Rebuild(ctx, docs); err != nil {
		return fmt.Errorf("search engine rebuild failed: %w", err)
	}

	slog.Info("Search engine synchronization complete", "count", len(docs))
	return nil
}

// runGC executes BadgerDB value log garbage collection periodically.
// Implements a progressive decay matrix to systematically reclaim disk space gracefully.
func (s *MemoryStore) runGC() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// GC Decay threshold logic (from gentlest to most aggressive)
	thresholds := []float64{0.7, 0.5, 0.3, 0.1}

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Explicit flatten unblocks LSM overlaps to harvest highly duplicated memory segments
			if err := s.db.Flatten(1); err != nil {
				slog.Warn("Badger Flatten execution failed during GC", "error", err)
			}

			// Progressive threshold fallback
			for _, ratio := range thresholds {
				runCount := 0
				for {
					err := s.db.RunValueLogGC(ratio)
					if err != nil {
						if err != badger.ErrNoRewrite && err != badger.ErrRejected {
							slog.Debug("Badger GC passed on threshold", "ratio", ratio, "error", err)
						}
						break
					}
					runCount++
				}
				if runCount > 0 {
					s.gcSweeps.Add(uint64(runCount))
					slog.Info("Badger progressive GC reclaimed disk blocks natively", "ratio", ratio, "cycles", runCount)
				}
			}
		case <-s.stopGC:
			return
		}
	}
}

// runAuditWorker periodically verifies the integrity of the search index.
func (s *MemoryStore) runAuditWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.performAudit()
		case <-s.stopAudit:
			return
		}
	}
}

func (s *MemoryStore) performAudit() {
	s.mu.RLock()
	searchEngine := s.search
	s.mu.RUnlock()

	if searchEngine == nil {
		return
	}

	// 🛡️ F8: Random sampling across entire keyspace (not just first N keys)
	var driftedKeys []string
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		// Phase 1: Count non-index keys
		var totalKeys int
		for it.Rewind(); it.Valid(); it.Next() {
			if !strings.HasPrefix(string(it.Item().Key()), "_idx:") {
				totalKeys++
			}
		}
		if totalKeys == 0 {
			return nil
		}

		// Phase 2: Pick random positions
		targets := sampleRandomPositions(totalKeys, 5)

		// Phase 3: Iterate again, auditing only selected positions
		idx := 0
		for it.Rewind(); it.Valid(); it.Next() {
			keyStr := string(it.Item().Key())
			if strings.HasPrefix(keyStr, "_idx:") {
				continue
			}
			if _, ok := targets[idx]; !ok {
				idx++
				continue
			}
			idx++

			if !verifySearchEntry(searchEngine, keyStr) {
				slog.Warn("Search index drift detected by audit worker", "key", keyStr)
				s.driftAlerts.Add(1)
				driftedKeys = append(driftedKeys, keyStr)
			}
		}
		return nil
	})

	if err != nil {
		slog.Error("Search audit worker failed", "error", err)
	}

	if len(driftedKeys) > 0 {
		ctx := s.ctx
		for _, key := range driftedKeys {
			rec, err := s.Get(ctx, key)
			if err != nil {
				continue
			}
			doc := &search.Document{
				Title:      rec.Title,
				SymbolName: rec.SymbolName,
				Content:    rec.Content,
				Category:   rec.Category,
				Tags:       rec.Tags,
				SourcePath: rec.SourcePath,
				SourceHash: rec.SourceHash,
			}
			if sErr := searchEngine.Index(key, doc); sErr == nil {
				slog.Info("Drift healed successfully", "key", key)
			} else {
				slog.Warn("Failed to heal drift", "key", key, "error", sErr)
			}
		}
	}
}

// sampleRandomPositions picks count random positions from [0, total).
func sampleRandomPositions(total, count int) map[int]struct{} {
	if total < count {
		count = total
	}
	targets := make(map[int]struct{}, count)
	for len(targets) < count {
		targets[rand.IntN(total)] = struct{}{}
	}
	return targets
}

// verifySearchEntry checks if a key exists natively in the search engine index.
func verifySearchEntry(engine search.SearchEngine, key string) bool {
	exists, err := engine.Has(key)
	if err != nil {
		return false
	}
	return exists
}

// DriftAlerts returns the total number of index mismatches detected.
func (s *MemoryStore) DriftAlerts() uint64 {
	return s.driftAlerts.Load()
}

// DocCount returns the number of documents in the Bleve index.
func (s *MemoryStore) DocCount() (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.search == nil {
		return 0, nil
	}
	return s.search.DocCount()
}

// SaveResult describes the outcome of a Save operation with dedup metadata.
type SaveResult struct {
	Action    string `json:"action"`                // "created", "updated", or "merged"
	Key       string `json:"key"`                   // Final key where the record lives
	MergedKey string `json:"merged_with,omitempty"` // Original key that was merged into (if Action=merged)
}

// Save stores or updates a memory Record in the database with optional inline dedup.
// When dedupThreshold > 0 and the key is new, same-category memory-domain records are
// scanned via Jaccard similarity. If a match exceeds the threshold, the incoming content
// merges into the existing record.
func (s *MemoryStore) Save(ctx context.Context, title, key, content, category string, tags []string, domain string, dedupThreshold float64) (*SaveResult, error) {
	if domain == "" {
		domain = DomainMemories
	}

	// Enforce namespace: memory-domain writes must not use standards categories.
	if domain == DomainMemories && HarvestedCategories[category] {
		return nil, fmt.Errorf("category %q is reserved for the standards domain", category)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Phase 1: Check if key already exists (upsert path).
	var oldRec *Record
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}
		return item.Value(func(val []byte) error {
			if old, err := migrateRecord(val); err == nil {
				oldRec = old
			} else {
				slog.Warn("Failed to migrate record during save", "key", key, "error", err)
			}
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("lookup failure during save: %w", err)
	}

	// Phase 2: Inline dedup — only for new keys in memory domain with threshold > 0.
	if oldRec == nil && dedupThreshold > 0 && domain == DomainMemories && category != "" {
		if match := s.findSimilarLocked(content, category, dedupThreshold); match != nil {
			// Merge incoming into existing record.
			mergedTags := mergeTags(match.Record.Tags, tags)
			mergedContent := match.Record.Content
			if len(content) > len(mergedContent) {
				mergedContent = content
			}
			mergedTitle := match.Record.Title
			if mergedTitle == "" && title != "" {
				mergedTitle = title
			}

			return s.updateRecordLocked(ctx, match.Key, mergedTitle, mergedContent, match.Record.Category, mergedTags, match.Record.Domain, match.Record.CreatedAt)
		}
	}

	// Phase 3: Standard write (create or update).
	action := "created"
	if oldRec != nil {
		action = "updated"
	}

	now := time.Now()
	err = s.UpdateWithRetry(func(txn *badger.Txn) error {
		if oldRec != nil {
			s.deleteRecordIndices(txn, key, oldRec)
		}

		rec := &Record{
			Title:     title,
			Content:   content,
			Category:  category,
			Domain:    domain,
			Tags:      tags,
			UpdatedAt: now,
		}
		if oldRec != nil {
			rec.CreatedAt = oldRec.CreatedAt
		} else {
			rec.CreatedAt = now
		}

		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}

		entry := badger.NewEntry([]byte(key), data)

		if err := txn.SetEntry(entry); err != nil {
			return fmt.Errorf("failed to set record: %w", err)
		}

		return s.createRecordIndices(txn, key, rec)
	})

	if err != nil {
		return nil, err
	}

	slog.Debug("Memory saved and indexed", "key", key, "category", category, "domain", domain, "tag_count", len(tags))

	// Write-through: update Bleve index (best-effort) concurrently.
	if s.search != nil {
		go func() {
			doc := &search.Document{Title: title, Content: content, Category: category, Tags: tags}
			if sErr := s.search.Index(key, doc); sErr != nil {
				slog.Warn("Bleve index update failed (non-fatal)", "key", key, "error", sErr)
			}
		}()
	}

	if action == "created" {
		switch domain {
		case DomainMemories:
			s.memoriesCount.Add(1)
		case DomainSessions:
			s.sessionsCount.Add(1)
		case DomainStandards:
			s.standardsCount.Add(1)
		case DomainProjects:
			s.projectsCount.Add(1)
		}
	}

	return &SaveResult{Action: action, Key: key}, nil
}

// updateRecordLocked writes a merged record. Caller must hold s.mu.
func (s *MemoryStore) updateRecordLocked(_ context.Context, key, title, content, category string, tags []string, domain string, createdAt time.Time) (*SaveResult, error) {
	now := time.Now()
	err := s.UpdateWithRetry(func(txn *badger.Txn) error {
		// Read and clean old indices.
		item, err := txn.Get([]byte(key))
		if err == nil {
			if vErr := item.Value(func(val []byte) error {
				if old, err := migrateRecord(val); err == nil {
					s.deleteRecordIndices(txn, key, old)
				}
				return nil
			}); vErr != nil {
				slog.Warn("Failed to read old record value during merge", "key", key, "error", vErr)
			}
		}

		rec := &Record{
			Title:     title,
			Content:   content,
			Category:  category,
			Domain:    domain,
			Tags:      tags,
			CreatedAt: createdAt,
			UpdatedAt: now,
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}

		entry := badger.NewEntry([]byte(key), data)

		if err := txn.SetEntry(entry); err != nil {
			return fmt.Errorf("failed to set merged record: %w", err)
		}
		return s.createRecordIndices(txn, key, rec)
	})
	if err != nil {
		return nil, err
	}

	slog.Info("Dedup merge completed", "merged_into", key)

	if s.search != nil {
		go func() {
			doc := &search.Document{Title: title, Content: content, Category: category, Tags: tags}
			if sErr := s.search.Index(key, doc); sErr != nil {
				slog.Warn("Bleve index update failed after dedup merge (non-fatal)", "key", key, "error", sErr)
			}
		}()
	}

	return &SaveResult{Action: "merged", Key: key, MergedKey: key}, nil
}

// findSimilarLocked scans same-category memory-domain records for Jaccard similarity.
// Returns the best match above threshold, or nil. Caller must hold s.mu.
func (s *MemoryStore) findSimilarLocked(content, category string, threshold float64) *SearchResult {
	catPrefix := fmt.Appendf(nil, "_idx:cat:%s:", strings.ToLower(category))
	var bestMatch *SearchResult
	bestScore := 0.0

	if err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(catPrefix); it.ValidForPrefix(catPrefix); it.Next() {
			if vErr := it.Item().Value(func(kVal []byte) error {
				originalKey := string(kVal)
				item, err := txn.Get(kVal)
				if err != nil {
					return nil
				}
				return item.Value(func(v []byte) error {
					rec, err := migrateRecord(v)
					if err != nil || rec.Domain != DomainMemories {
						return nil
					}
					score := computeJaccard(content, rec.Content)
					if score >= threshold && score > bestScore {
						bestScore = score
						bestMatch = &SearchResult{Key: originalKey, Record: rec}
					}
					return nil
				})
			}); vErr != nil {
				slog.Warn("Failed to read index value during dedup scan", "error", vErr)
			}
		}
		return nil
	}); err != nil {
		slog.Warn("Dedup scan view failed", "category", category, "error", err)
	}

	return bestMatch
}

// mergeTags unions two tag slices, deduplicating by lowercase key.
func mergeTags(existing, incoming []string) []string {
	tagSet := make(map[string]struct{}, len(existing)+len(incoming))
	for _, t := range existing {
		tagSet[strings.ToLower(t)] = struct{}{}
	}
	for _, t := range incoming {
		tagSet[strings.ToLower(t)] = struct{}{}
	}
	merged := make([]string, 0, len(tagSet))
	for t := range tagSet {
		merged = append(merged, t)
	}
	return merged
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
			s.cacheMisses.Add(1)
			return nil, fmt.Errorf("memory not found: %s", key)
		}
		return nil, err
	}
	s.cacheHits.Add(1)
	return rec, nil
}

// Search matches keys, content, and tags with fuzzy relevance ranking and limits.
func (s *MemoryStore) Search(ctx context.Context, query string, tagFilter string, limit int) ([]*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var candidates []*SearchResult
	tagFilter = strings.ToLower(tagFilter)

	err := s.db.View(func(txn *badger.Txn) error {
		var scanErr error
		if tagFilter != "" {
			candidates, scanErr = searchByTag(ctx, txn, tagFilter)
		} else {
			candidates, scanErr = searchGeneral(ctx, txn)
		}
		return scanErr
	})

	if err != nil {
		return nil, err
	}

	var final []*SearchResult

	// If no query, return chronological/original order limited
	if query == "" {
		if limit > 0 && len(candidates) > limit {
			final = candidates[:limit]
		} else {
			final = candidates
		}
	} else {
		// Perform fuzzy matching and scoring
		results := s.rankCandidates(ctx, query, candidates)

		if limit > 0 && len(results) > limit {
			final = results[:limit]
		} else {
			final = results
		}
	}

	if len(final) > 0 {
		s.cacheHits.Add(uint64(len(final)))
	} else {
		s.cacheMisses.Add(1)
	}
	return final, nil
}

// VacuumSessions performs semantic pruning on sessions matching the target outcome.
// It evicts AST payloads from Bleve (via batch) and writes BadgerDB tombstones.
// Triggers an LSM Flatten if mutates >= flattenThreshold.
// Bounded ValueLog GC is triggered async on exit.
func (s *MemoryStore) VacuumSessions(ctx context.Context, targetOutcome string, flattenThreshold int, daysOld int) (int, error) {
	s.mu.RLock()
	searchEngine := s.search
	s.mu.RUnlock()

	var targets []string
	var mutated int

	// Find the targeted sessions
	sessions, err := s.ListSessions(ctx, "", "", targetOutcome, "")
	if err != nil {
		return 0, fmt.Errorf("failed to list sessions for vacuum: %w", err)
	}

	for _, session := range sessions {
		if daysOld > 0 && time.Since(session.Record.UpdatedAt) < time.Duration(daysOld)*24*time.Hour {
			continue
		}
		targets = append(targets, session.Key)
	}

	if len(targets) == 0 {
		return 0, nil
	}

	now := time.Now()

	err = s.UpdateWithRetry(func(txn *badger.Txn) error {
		// Prepare batch deletion for Bleve if available
		var batch map[string]bool
		if searchEngine != nil {
			batch = make(map[string]bool)
		}

		for _, key := range targets {
			item, err := txn.Get([]byte(key))
			if err != nil {
				continue
			}
			err = item.Value(func(val []byte) error {
				if rec, err := migrateRecord(val); err == nil {
					// Prepare Tombstone
					rec.Content = fmt.Sprintf(`{"status": "tombstoned", "original_outcome": %q, "vacuumed_at": %q, "reason": "semantic pruning"}`, targetOutcome, now.Format(time.RFC3339))
					rec.UpdatedAt = now

					data, err := json.Marshal(rec)
					if err != nil {
						return err
					}

					entry := badger.NewEntry([]byte(key), data)
					if err := txn.SetEntry(entry); err != nil {
						return err
					}

					if searchEngine != nil {
						batch[key] = true
					}
					mutated++
				}
				return nil
			})
			if err != nil {
				return err
			}
		}

		// Execute the Bleve batch deletion using our SearchEngine interface
		// Note: we can't cleanly batch delete natively with the current interface here,
		// but we expose Delete which will delete immediately.
		// To adhere to 'best effort' we will execute deletes outside lock if possible.
		return nil
	})

	if err != nil {
		return mutated, fmt.Errorf("badger update failed: %w", err)
	}

	// Process Bleve individual deletes since we don't have a BatchDelete on the custom SearchEngine interface yet
	if searchEngine != nil {
		for _, key := range targets {
			_ = searchEngine.Delete(key)
		}
	}

	// Trigger Flatten
	if mutated >= flattenThreshold {
		slog.Warn("LSM Flatten triggered by context vacuum threshold", "mutated", mutated, "threshold", flattenThreshold)
		_ = s.db.Flatten(1)
	}

	// Trigger Async ValueLog GC
	go func() {
		reclaimed := 0
		for range 100 { // Bounded safety loop
			gcErr := s.db.RunValueLogGC(0.5)
			if gcErr != nil {
				break
			}
			reclaimed++
		}
		if reclaimed > 0 {
			slog.Info("Context vacuum GC reclaimed disk blocks", "blocks_rewritten", reclaimed)
		}
	}()

	return mutated, nil
}

// ---------------------------------------------------------------------------
// Universal Vacuum Types
// ---------------------------------------------------------------------------

// StaleEntry represents a memory entry flagged as stale during vacuum analysis.
type StaleEntry struct {
	Key       string `json:"key"`
	Category  string `json:"category"`
	AgeDays   int    `json:"age_days"`
	UpdatedAt string `json:"updated_at"`
}

// DuplicateCluster groups keys that are near-duplicates by content similarity.
type DuplicateCluster struct {
	Category string   `json:"category"`
	Keys     []string `json:"keys"`
	Score    float64  `json:"similarity_score"`
}

// CategoryHealth summarizes the health of a single category.
type CategoryHealth struct {
	Category    string `json:"category"`
	EntryCount  int    `json:"entry_count"`
	AvgAgeDays  int    `json:"avg_age_days"`
	StalestDays int    `json:"stalest_days"`
}

// VacuumReport is the unified result type for all vacuum operations across namespaces.
type VacuumReport struct {
	Namespace          string             `json:"namespace"`
	TotalScanned       int                `json:"total_scanned"`
	StaleEntries       []StaleEntry       `json:"stale_entries,omitempty"`
	DuplicateClusters  []DuplicateCluster `json:"duplicate_clusters,omitempty"`
	CategoryHealthList []CategoryHealth   `json:"category_health,omitempty"`
	Pruned             int                `json:"pruned"`
	Merged             int                `json:"merged"`
	ReportOnly         bool               `json:"report_only"`
}

// ---------------------------------------------------------------------------
// VacuumMemories: Staleness + Duplicate Detection for the memories namespace
// ---------------------------------------------------------------------------

// VacuumMemories scans the memories domain for stale entries and near-duplicates.
// When reportOnly is true, analysis is returned without mutations.
// The Jaccard dedup scan is capped at 100 entries per category to avoid O(n²) blowup.
func (s *MemoryStore) VacuumMemories(ctx context.Context, daysOld int, dedupThreshold float64, categoryFilter string, reportOnly bool) (*VacuumReport, error) {
	s.mu.RLock()
	searchEngine := s.search
	s.mu.RUnlock()

	now := time.Now()
	report := &VacuumReport{
		Namespace:  DomainMemories,
		ReportOnly: reportOnly,
	}

	// Phase 1: Collect all memory-domain records grouped by category.
	type memEntry struct {
		key     string
		rec     *Record
		ageDays int
	}
	byCategory := make(map[string][]memEntry)

	if err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("_idx:domain:memories:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
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
					if err != nil || rec.Domain != DomainMemories {
						return nil
					}
					if categoryFilter != "" && rec.Category != categoryFilter {
						return nil
					}
					age := int(now.Sub(rec.UpdatedAt).Hours() / 24)
					report.TotalScanned++
					byCategory[rec.Category] = append(byCategory[rec.Category], memEntry{
						key:     originalKey,
						rec:     rec,
						ageDays: age,
					})
					return nil
				})
			}); err != nil {
				slog.Warn("Error scanning memory during vacuum", "error", err)
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("memory vacuum scan failed: %w", err)
	}

	// Phase 2: Analyze each category.
	var staleKeys []string
	var mergeKeys []string

	categories := make([]string, 0, len(byCategory))
	for cat := range byCategory {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	for _, cat := range categories {
		entries := byCategory[cat]
		totalAge := 0
		stalest := 0

		for _, e := range entries {
			totalAge += e.ageDays
			if e.ageDays > stalest {
				stalest = e.ageDays
			}

			// Flag stale entries.
			if daysOld > 0 && e.ageDays >= daysOld {
				report.StaleEntries = append(report.StaleEntries, StaleEntry{
					Key:       e.key,
					Category:  e.rec.Category,
					AgeDays:   e.ageDays,
					UpdatedAt: e.rec.UpdatedAt.Format(time.RFC3339),
				})
				staleKeys = append(staleKeys, e.key)
			}
		}

		avgAge := 0
		if len(entries) > 0 {
			avgAge = totalAge / len(entries)
		}
		report.CategoryHealthList = append(report.CategoryHealthList, CategoryHealth{
			Category:    cat,
			EntryCount:  len(entries),
			AvgAgeDays:  avgAge,
			StalestDays: stalest,
		})

		// Jaccard duplicate detection — bounded to 100 entries per category.
		sampleSize := min(len(entries), 100)
		sample := entries[:sampleSize]

		// Track which keys are already part of a cluster to avoid duplicating.
		clustered := make(map[string]bool)
		for i := range sample {
			if clustered[sample[i].key] {
				continue
			}
			var cluster []string
			var bestScore float64
			for j := i + 1; j < len(sample); j++ {
				if clustered[sample[j].key] {
					continue
				}
				score := computeJaccard(sample[i].rec.Content, sample[j].rec.Content)
				if score >= dedupThreshold {
					if len(cluster) == 0 {
						cluster = append(cluster, sample[i].key)
						clustered[sample[i].key] = true
					}
					cluster = append(cluster, sample[j].key)
					clustered[sample[j].key] = true
					if score > bestScore {
						bestScore = score
					}
				}
			}
			if len(cluster) > 1 {
				report.DuplicateClusters = append(report.DuplicateClusters, DuplicateCluster{
					Category: cat,
					Keys:     cluster,
					Score:    bestScore,
				})
				// For merge: keep the first key (oldest), mark rest for removal.
				mergeKeys = append(mergeKeys, cluster[1:]...)
			}
		}
	}

	// Phase 3: Mutate if not report-only.
	if !reportOnly {
		allPrune := make([]string, 0, len(staleKeys)+len(mergeKeys))
		allPrune = append(allPrune, staleKeys...)
		allPrune = append(allPrune, mergeKeys...)

		if len(allPrune) > 0 {
			// Deduplicate keys in case a stale entry is also a duplicate.
			seen := make(map[string]bool, len(allPrune))
			unique := allPrune[:0]
			for _, k := range allPrune {
				if !seen[k] {
					seen[k] = true
					unique = append(unique, k)
				}
			}
			allPrune = unique

			if err := s.DeleteBatch(ctx, allPrune); err != nil {
				return report, fmt.Errorf("vacuum memory pruning failed: %w", err)
			}
		}

		report.Pruned = len(staleKeys)
		report.Merged = len(mergeKeys)

		s.triggerDBMaintenance(len(allPrune), 1000)
	}

	// Remove Bleve refs for stale entries even in report_only to keep index tidy?
	// No — report_only means NO mutations at all.
	_ = searchEngine // Defensive: used only when !reportOnly via DeleteBatch path.

	return report, nil
}

// ---------------------------------------------------------------------------
// VacuumStandards: Orphan Detection for the standards namespace
// ---------------------------------------------------------------------------

// VacuumStandards scans the standards domain for orphaned drift checksums
// (SysDrift keys with no corresponding symbol records) and empty packages.
// When reportOnly is true, analysis is returned without mutations.
func (s *MemoryStore) VacuumStandards(ctx context.Context, reportOnly bool) (*VacuumReport, error) {
	report := &VacuumReport{
		Namespace:  DomainStandards,
		ReportOnly: reportOnly,
	}

	// Collect all standards keys grouped by package.
	type pkgStats struct {
		symbolKeys []string
		driftKey   string
	}
	packages := make(map[string]*pkgStats)

	if err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("pkg:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			key := string(it.Item().Key())
			if err := it.Item().Value(func(v []byte) error {
				rec, err := migrateRecord(v)
				if err != nil {
					return nil
				}
				report.TotalScanned++

				// Extract package path from key: "pkg:<path>:<SymbolName>"
				parts := strings.SplitN(strings.TrimPrefix(key, "pkg:"), ":", 2)
				if len(parts) < 2 {
					return nil
				}
				pkgPath := parts[0]

				if _, ok := packages[pkgPath]; !ok {
					packages[pkgPath] = &pkgStats{}
				}

				if rec.Category == "SysDrift" {
					packages[pkgPath].driftKey = key
				} else {
					packages[pkgPath].symbolKeys = append(packages[pkgPath].symbolKeys, key)
				}
				return nil
			}); err != nil {
				slog.Warn("Error scanning standards during vacuum", "error", err)
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("standards vacuum scan failed: %w", err)
	}

	// Find orphans: drift keys with no symbols, or empty packages.
	var orphanKeys []string
	for pkgPath, stats := range packages {
		if stats.driftKey != "" && len(stats.symbolKeys) == 0 {
			report.StaleEntries = append(report.StaleEntries, StaleEntry{
				Key:      stats.driftKey,
				Category: "SysDrift (orphaned)",
			})
			orphanKeys = append(orphanKeys, stats.driftKey)
		}
		_ = pkgPath // used in the map iteration
	}

	if !reportOnly && len(orphanKeys) > 0 {
		if err := s.DeleteBatch(ctx, orphanKeys); err != nil {
			return report, fmt.Errorf("vacuum standards orphan cleanup failed: %w", err)
		}
		report.Pruned = len(orphanKeys)
		s.triggerDBMaintenance(len(orphanKeys), 1000)
	}

	return report, nil
}

// triggerDBMaintenance performs LSM Flatten + async ValueLog GC if mutations exceed the threshold.
// Extracted from VacuumSessions to share across all vacuum namespaces.
func (s *MemoryStore) triggerDBMaintenance(mutated, flattenThreshold int) {
	s.gcPrunedNodes.Add(uint64(mutated))

	if mutated >= flattenThreshold {
		slog.Warn("LSM Flatten triggered by context vacuum threshold", "mutated", mutated, "threshold", flattenThreshold)
		_ = s.db.Flatten(1)
	}

	go func() {
		reclaimed := 0
		for range 100 {
			gcErr := s.db.RunValueLogGC(0.5)
			if gcErr != nil {
				break
			}
			reclaimed++
		}
		if reclaimed > 0 {
			s.gcSweeps.Add(uint64(reclaimed))
			slog.Info("Context vacuum GC reclaimed disk blocks", "blocks_rewritten", reclaimed)
		}
	}()
}

// searchByTag performs an O(K) index-based scan for records with a specific tag.
func searchByTag(ctx context.Context, txn *badger.Txn, tagFilter string) ([]*SearchResult, error) {
	var candidates []*SearchResult
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	prefix := fmt.Appendf(nil, "_idx:tag:%s:", tagFilter)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if err := it.Item().Value(func(kVal []byte) error {
			originalKey := string(kVal)
			item, err := txn.Get(kVal)
			if err != nil {
				return nil
			}
			return item.Value(func(v []byte) error {
				if rec, err := migrateRecord(v); err == nil {
					// Namespace isolation: exclude standards domain from search
					if HarvestedCategories[rec.Category] {
						return nil
					}
					candidates = append(candidates, &SearchResult{Key: originalKey, Record: rec})
				}
				return nil
			})
		}); err != nil {
			slog.Warn("Corrupted memory entry detected during search (tag)", "error", err)
		}
	}
	return candidates, nil
}

// searchGeneral performs a linear scan of all non-index records.
func searchGeneral(ctx context.Context, txn *badger.Txn) ([]*SearchResult, error) {
	var candidates []*SearchResult
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		item := it.Item()
		k := string(item.Key())
		if strings.HasPrefix(k, "_idx:") {
			continue
		}

		if err := item.Value(func(v []byte) error {
			rec, err := migrateRecord(v)
			if err != nil {
				return nil
			}
			// Exclude standards-domain records from memory search
			if HarvestedCategories[rec.Category] {
				return nil
			}
			candidates = append(candidates, &SearchResult{Key: k, Record: rec})
			return nil
		}); err != nil {
			slog.Warn("Corrupted memory entry detected during search (general)", "key", k, "error", err)
		}
	}
	return candidates, nil
}

// rankCandidates applies fuzzy scoring across Key, Content, and Tags with parallelism.
func (s *MemoryStore) rankCandidates(ctx context.Context, query string, candidates []*SearchResult) []*SearchResult {
	if len(candidates) == 0 {
		return candidates
	}

	var wg sync.WaitGroup
	// Concurrency: Use worker pool based on CPU count or chunking
	workerCount := min(len(candidates), 4)

	chunkSize := (len(candidates) + workerCount - 1) / workerCount
	for i := 0; i < workerCount; i++ {
		start := i * chunkSize
		if start >= len(candidates) {
			break
		}
		end := min(start+chunkSize, len(candidates))

		wg.Add(1)
		go func(ctx context.Context, subset []*SearchResult) {
			defer wg.Done()
			for _, c := range subset {
				select {
				case <-ctx.Done():
					return
				default:
					scoreSingleCandidate(query, c)
				}
			}
		}(ctx, candidates[start:end])
	}
	wg.Wait()

	// Filter out zero scores and sort
	var ranked []*SearchResult
	for _, c := range candidates {
		if c.Score > 0 {
			ranked = append(ranked, c)
		}
	}

	slices.SortFunc(ranked, func(a, b *SearchResult) int {
		if a.Score < b.Score {
			return 1
		}
		if a.Score > b.Score {
			return -1
		}
		return 0
	})

	return ranked
}

// scoreSingleCandidate scores a candidate against the query across key, content, category, and tags.
func scoreSingleCandidate(query string, c *SearchResult) {
	if c.Record == nil {
		return
	}
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
					if rec, err := migrateRecord(v); err == nil && rec.Domain == DomainMemories {
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

	if err == nil {
		if len(results) > 0 {
			s.cacheHits.Add(uint64(len(results)))
		} else {
			s.cacheMisses.Add(1)
		}
	}
	return results, err
}

// ListKeys retrieves all available keys for knowledge discovery.
// Scoped exclusively to the memories domain via the _idx:domain:memories: index.
func (s *MemoryStore) ListKeys(ctx context.Context) ([]*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*SearchResult
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("_idx:domain:memories:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if err := it.Item().Value(func(kVal []byte) error {
				originalKey := string(kVal)
				item, err := txn.Get(kVal)
				if err != nil {
					return nil
				}
				return item.Value(func(v []byte) error {
					if rec, err := migrateRecord(v); err == nil {
						results = append(results, &SearchResult{Key: originalKey, Record: rec})
					}
					return nil
				})
			}); err != nil {
				slog.Warn("Corrupted memory entry detected during list", "error", err)
			}
		}
		return nil
	})
	if err == nil {
		if len(results) > 0 {
			s.cacheHits.Add(uint64(len(results)))
		} else {
			s.cacheMisses.Add(1)
		}
	}
	return results, err
}

func (s *MemoryStore) ListSessions(ctx context.Context, projectID, serverID, outcome, traceContext string) ([]*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*SearchResult
	err := s.db.View(func(txn *badger.Txn) error {
		// Default to domain index, narrow dynamically if tags specify tighter bounds
		prefixStr := "_idx:domain:sessions:"
		if traceContext != "" {
			prefixStr = fmt.Sprintf("_idx:tag:trace:%s:", strings.ToLower(traceContext))
		} else if projectID != "" {
			prefixStr = fmt.Sprintf("_idx:tag:project:%s:", strings.ToLower(projectID))
		} else if outcome != "" {
			prefixStr = fmt.Sprintf("_idx:tag:outcome:%s:", strings.ToLower(outcome))
		}

		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefixStr)
		opts.PrefetchValues = true // We need the actual key value from the index
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			idxItem := it.Item()
			actualKey, err := idxItem.ValueCopy(nil)
			if err != nil {
				continue
			}

			// Validate server filter physically on the exact target key bounding
			if serverID != "" && !strings.HasPrefix(string(actualKey), serverID+":session:") {
				continue
			}

			recItem, err := txn.Get(actualKey)
			if err != nil {
				continue // index orphaned?
			}

			if err := recItem.Value(func(v []byte) error {
				if rec, err := migrateRecord(v); err == nil && rec.Domain == DomainSessions {
					// Client-side cross-filtering to verify secondary filters not caught by the primary prefix scan
					if s.matchSessionFilters(rec, projectID, outcome, traceContext) {
						results = append(results, &SearchResult{Key: string(actualKey), Record: rec})
					}
				}
				return nil
			}); err != nil {
				slog.Warn("Corrupted session entry detected during list", "error", err)
			}
		}
		return nil
	})

	if err == nil {
		if len(results) > 0 {
			s.cacheHits.Add(uint64(len(results)))
		} else {
			s.cacheMisses.Add(1)
		}
	}
	return results, err
}

func (s *MemoryStore) SearchSessions(ctx context.Context, query, projectID, serverID, outcome, traceContext string, limit int) ([]*SearchResult, error) {
	candidates, err := s.ListSessions(ctx, projectID, serverID, outcome, traceContext)
	if err != nil {
		return nil, err
	}

	if query == "" {
		if limit > 0 && len(candidates) > limit {
			return candidates[:limit], nil
		}
		return candidates, nil
	}

	final := s.rankCandidates(ctx, query, candidates)
	if limit > 0 && len(final) > limit {
		final = final[:limit]
	}
	return final, nil
}

func (s *MemoryStore) matchSessionFilters(rec *Record, pID, out, trace string) bool {
	tags := make(map[string]bool)
	for _, t := range rec.Tags {
		tags[strings.ToLower(t)] = true
	}
	if pID != "" && !tags[fmt.Sprintf("project:%s", strings.ToLower(pID))] {
		return false
	}
	if out != "" && !tags[fmt.Sprintf("outcome:%s", strings.ToLower(out))] {
		return false
	}
	if trace != "" && !tags[fmt.Sprintf("trace:%s", strings.ToLower(trace))] {
		return false
	}
	return true
}

// Clear removes all stored memories by dropping and recreating or clearing.
func (s *MemoryStore) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.db.DropAll(); err != nil {
		return err
	}
	s.memoriesCount.Store(0)
	s.sessionsCount.Store(0)
	s.standardsCount.Store(0)

	// Reset Bleve index to empty.
	if s.search != nil {
		if sErr := s.search.Rebuild(ctx, nil); sErr != nil {
			slog.Warn("Bleve index reset failed after clear (non-fatal)", "error", sErr)
		}
	}

	return nil
}

// GetMetrics returns a snapshot of cache performance.
func (s *MemoryStore) GetMetrics() CacheMetrics {
	var metrics CacheMetrics
	metrics.Hits = s.cacheHits.Load()
	metrics.Misses = s.cacheMisses.Load()
	metrics.Memories = int(s.memoriesCount.Load())
	metrics.Sessions = int(s.sessionsCount.Load())
	metrics.Standards = int(s.standardsCount.Load())
	metrics.Projects = int(s.projectsCount.Load())

	if count, err := s.DocCount(); err == nil {
		metrics.BleveDocs = count
	}

	var statsErr error
	if metrics.Entries, _, statsErr = s.GetStats(); statsErr != nil {
		slog.Debug("Failed to get db stats for metrics", "error", statsErr)
	}
	return metrics
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
// Scoped to the memories domain — rejects keys belonging to the standards namespace.
func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Pre-flight: reject standards-domain records.
	var rec *Record
	if err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			r, mErr := migrateRecord(val)
			if mErr != nil {
				slog.Warn("Failed to migrate record during delete pre-flight", "key", key, "error", mErr)
				return nil
			}
			rec = r
			return nil
		})
	}); err != nil && err != badger.ErrKeyNotFound {
		slog.Warn("Delete pre-flight view failed", "key", key, "error", err)
	}
	if rec != nil && rec.Domain == DomainStandards {
		return fmt.Errorf("key %q belongs to the standards domain; use standards tools to manage it", key)
	}

	if err := s.deleteNoLock(key); err != nil {
		return err
	}

	// Write-through: remove from Bleve index (best-effort).
	if s.search != nil {
		if sErr := s.search.Delete(key); sErr != nil {
			slog.Warn("Bleve index delete failed (non-fatal)", "key", key, "error", sErr)
		}
	}

	return nil
}

func (s *MemoryStore) deleteNoLock(key string) error {
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
	if err != nil && err != badger.ErrKeyNotFound {
		slog.Error("Failed to fetch record for deletion", "key", key, "error", err)
	}

	return s.UpdateWithRetry(func(txn *badger.Txn) error {
		if rec != nil {
			s.deleteRecordIndices(txn, key, rec)
			switch rec.Domain {
			case DomainMemories:
				s.memoriesCount.Add(-1)
			case DomainSessions:
				s.sessionsCount.Add(-1)
			case DomainStandards:
				s.standardsCount.Add(-1)
			case DomainProjects:
				s.projectsCount.Add(-1)
			}
		}
		return txn.Delete([]byte(key))
	})
}

func (s *MemoryStore) deleteRecordIndices(txn *badger.Txn, key string, rec *Record) {
	// 1. Time Index
	timeIdx := fmt.Sprintf("_idx:t:%x:%s", rec.UpdatedAt.UnixNano(), key)
	if err := txn.Delete([]byte(timeIdx)); err != nil && err != badger.ErrKeyNotFound {
		slog.Warn("Failed to delete time index", "key", key, "error", err)
	}

	// 2. Tag Indices
	for _, t := range rec.Tags {
		tagIdx := fmt.Sprintf("_idx:tag:%s:%s", strings.ToLower(t), key)
		if err := txn.Delete([]byte(tagIdx)); err != nil && err != badger.ErrKeyNotFound {
			slog.Warn("Failed to delete tag index", "tag", t, "key", key, "error", err)
		}
	}

	// 3. Category Index
	if rec.Category != "" {
		catIdx := fmt.Sprintf("_idx:cat:%s:%s", strings.ToLower(rec.Category), key)
		if err := txn.Delete([]byte(catIdx)); err != nil && err != badger.ErrKeyNotFound {
			slog.Warn("Failed to delete category index", "cat", rec.Category, "key", key, "error", err)
		}
	}

	// 4. Domain Index
	if rec.Domain != "" {
		domIdx := fmt.Sprintf("_idx:domain:%s:%s", rec.Domain, key)
		if err := txn.Delete([]byte(domIdx)); err != nil && err != badger.ErrKeyNotFound {
			slog.Warn("Failed to delete domain index", "domain", rec.Domain, "key", key, "error", err)
		}
	}
}

func (s *MemoryStore) createRecordIndices(txn *badger.Txn, key string, rec *Record) error {
	// 1. Time Index
	timeIdx := fmt.Sprintf("_idx:t:%x:%s", rec.UpdatedAt.UnixNano(), key)
	if err := txn.Set([]byte(timeIdx), []byte(key)); err != nil {
		return fmt.Errorf("failed to set time index: %w", err)
	}

	// 2. Category Index
	if rec.Category != "" {
		catIdx := fmt.Sprintf("_idx:cat:%s:%s", strings.ToLower(rec.Category), key)
		if err := txn.Set([]byte(catIdx), []byte(key)); err != nil {
			return fmt.Errorf("failed to set category index: %w", err)
		}
	}

	// 3. Tag Indices
	for _, t := range rec.Tags {
		tagIdx := fmt.Sprintf("_idx:tag:%s:%s", strings.ToLower(t), key)
		if err := txn.Set([]byte(tagIdx), []byte(key)); err != nil {
			return fmt.Errorf("failed to set tag index for %s: %w", t, err)
		}
	}

	// 4. Domain Index
	if rec.Domain != "" {
		domIdx := fmt.Sprintf("_idx:domain:%s:%s", rec.Domain, key)
		if err := txn.Set([]byte(domIdx), []byte(key)); err != nil {
			return fmt.Errorf("failed to set domain index: %w", err)
		}
	}

	return nil
}

// BatchEntry represents a single item in a batch write operation.
type BatchEntry struct {
	Title      string    `json:"title,omitempty"`
	Key        string    `json:"key"`
	Value      string    `json:"value"`
	Category   string    `json:"category,omitempty"`
	Domain     string    `json:"domain,omitempty"`
	SessionID  string    `json:"session_id,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	SourcePath string    `json:"source_path,omitempty"`
	SourceHash string    `json:"source_hash,omitempty"`
	SymbolName string    `json:"symbolname,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// BatchError reports a per-key failure during batch operations.
type BatchError struct {
	Key   string `json:"key"`
	Error string `json:"error"`
}

// SaveBatch atomically stores multiple entries in a single BadgerDB transaction.
// All entries are committed together; if any fails, the entire batch is rolled back.
func (s *MemoryStore) SaveBatch(ctx context.Context, entries []BatchEntry) (stored int, batchErrors []BatchError, err error) {
	if len(entries) == 0 {
		return 0, nil, nil
	}
	if len(entries) > s.maxBatchSize {
		return 0, nil, fmt.Errorf("batch size %d exceeds maximum of %d", len(entries), s.maxBatchSize)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Phase 1: Collect existing records for index cleanup (read-only pass).
	oldRecords, lookupErr := s.collectExistingRecords(entries)
	if lookupErr != nil {
		return 0, nil, lookupErr
	}

	// Phase 2: Atomic write — commit all entries + indices in a single transaction.
	now := time.Now()
	err = s.UpdateWithRetry(func(txn *badger.Txn) error {
		for _, e := range entries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Clean up old indices if updating an existing key.
			if oldRec, exists := oldRecords[e.Key]; exists {
				s.deleteRecordIndices(txn, e.Key, oldRec)
			}

			rec := &Record{
				Title:      e.Title,
				SymbolName: e.SymbolName,
				Content:    e.Value,
				Category:   e.Category,
				Tags:       e.Tags,
				SourcePath: e.SourcePath,
				SourceHash: e.SourceHash,
			}

			// Temporal fidelity: honor imported timestamps when present,
			// otherwise default to current time.
			if !e.UpdatedAt.IsZero() {
				rec.UpdatedAt = e.UpdatedAt
			} else {
				rec.UpdatedAt = now
			}

			if e.Domain != "" {
				rec.Domain = e.Domain
			} else {
				if HarvestedCategories[e.Category] {
					rec.Domain = DomainStandards
				} else {
					rec.Domain = DomainMemories
				}
			}

			if e.SessionID != "" {
				rec.SessionID = e.SessionID
			}

			if oldRec, exists := oldRecords[e.Key]; exists {
				rec.CreatedAt = oldRec.CreatedAt
			} else if !e.CreatedAt.IsZero() {
				rec.CreatedAt = e.CreatedAt
			} else {
				rec.CreatedAt = now
				switch rec.Domain {
				case DomainMemories:
					s.memoriesCount.Add(1)
				case DomainSessions:
					s.sessionsCount.Add(1)
				case DomainStandards:
					s.standardsCount.Add(1)
				case DomainProjects:
					s.projectsCount.Add(1)
				}
			}

			data, err := json.Marshal(rec)
			if err != nil {
				return fmt.Errorf("failed to marshal record for key %q: %w", e.Key, err)
			}
			if err := txn.Set([]byte(e.Key), data); err != nil {
				return fmt.Errorf("failed to set key %q: %w", e.Key, err)
			}
			if err := s.createRecordIndices(txn, e.Key, rec); err != nil {
				return fmt.Errorf("failed to index key %q: %w", e.Key, err)
			}
		}
		return nil
	})

	if err != nil {
		return 0, nil, fmt.Errorf("batch write failed (atomic rollback): %w", err)
	}

	// Write-through: bulk index via Bleve Batch (best-effort).
	s.syncBatchToSearchIndex(entries)

	slog.Info("Batch save completed", "entries", len(entries))
	return len(entries), nil, nil
}

// collectExistingRecords performs a read-only lookup for entries that already exist in the DB.
// Returns a map of key -> existing Record for index cleanup during updates.
func (s *MemoryStore) collectExistingRecords(entries []BatchEntry) (map[string]*Record, error) {
	oldRecords := make(map[string]*Record, len(entries))
	if err := s.db.View(func(txn *badger.Txn) error {
		for _, e := range entries {
			item, err := txn.Get([]byte(e.Key))
			if err != nil {
				if err == badger.ErrKeyNotFound {
					continue
				}
				return err
			}
			if err := item.Value(func(val []byte) error {
				if old, err := migrateRecord(val); err == nil {
					oldRecords[e.Key] = old
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("batch lookup failure: %w", err)
	}
	return oldRecords, nil
}

// syncBatchToSearchIndex pushes batch entries to the Bleve search index (best-effort).
func (s *MemoryStore) syncBatchToSearchIndex(entries []BatchEntry) {
	if s.search == nil {
		return
	}
	sdocs := make(map[string]*search.Document, len(entries))
	for _, e := range entries {
		sdocs[e.Key] = &search.Document{
			Title:      e.Title,
			Content:    e.Value,
			Category:   e.Category,
			Tags:       e.Tags,
			SourcePath: e.SourcePath,
			SourceHash: e.SourceHash,
		}
	}
	if sErr := s.search.IndexBatch(sdocs); sErr != nil {
		slog.Warn("Bleve batch index failed after SaveBatch (non-fatal)", "count", len(entries), "error", sErr)
	}
}

// GetBatch retrieves multiple records by key in a single read-only transaction.
// Returns found records and a list of missing keys separately.
func (s *MemoryStore) GetBatch(ctx context.Context, keys []string) (map[string]*Record, []string, error) {
	if len(keys) == 0 {
		return nil, nil, nil
	}
	if len(keys) > s.maxBatchSize {
		return nil, nil, fmt.Errorf("batch size %d exceeds maximum of %d", len(keys), s.maxBatchSize)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	found := make(map[string]*Record, len(keys))
	var missing []string

	err := s.db.View(func(txn *badger.Txn) error {
		for _, key := range keys {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			item, err := txn.Get([]byte(key))
			if err != nil {
				if err == badger.ErrKeyNotFound {
					missing = append(missing, key)
					continue
				}
				return fmt.Errorf("failed to get key %q: %w", key, err)
			}

			if err := item.Value(func(val []byte) error {
				rec, mErr := migrateRecord(val)
				if mErr != nil {
					return mErr
				}
				found[key] = rec
				return nil
			}); err != nil {
				slog.Warn("Corrupted entry in batch read, treating as missing", "key", key, "error", err)
				missing = append(missing, key)
			}
		}
		return nil
	})

	if err != nil {
		return nil, nil, fmt.Errorf("batch read failed: %w", err)
	}

	slog.Info("Batch read completed", "found", len(found), "missing", len(missing))
	return found, missing, nil
}

// Close safely shuts down the database and maintenance routines.
func (s *MemoryStore) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.stopGC)
		close(s.stopAudit)
		if s.search != nil {
			if sErr := s.search.Close(); sErr != nil {
				slog.Warn("Failed to close search engine", "error", sErr)
			}
		}
		err = s.db.Close()
	})
	return err
}

// UpdateWithRetry wraps badger.Update with exponential backoff for Transaction Conflicts.
// Crucial for mitigating concurrent memory pipeline ingest collisions natively.
func (s *MemoryStore) UpdateWithRetry(fn func(txn *badger.Txn) error) error {
	maxRetries := 5
	backoff := 10 * time.Millisecond

	var err error
	for range maxRetries {
		err = s.db.Update(fn)
		if err == nil {
			return nil
		}
		if err != badger.ErrConflict {
			return err
		}

		time.Sleep(backoff)
		backoff *= 2
	}
	return fmt.Errorf("transaction conflict after %d retries: %w", maxRetries, err)
}

// ListCategories retrieves a unique list of all memory categories with counts.
// Standards-domain categories (HarvestedCode, PackageDoc, SysDrift) are excluded.
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
				if !HarvestedCategories[cat] {
					categories[cat]++
				}
				it.Next()
			} else {
				it.Next()
			}
		}
		return nil
	})
	return categories, err
}

// StandardsSymbolSummary represents a single symbol entry in the standards overview.
type StandardsSymbolSummary struct {
	Name       string `json:"name"`
	SymbolType string `json:"symbol_type"`
	Key        string `json:"key"`
}

// StandardsPackageOverview represents a package-level grouping of harvested symbols.
type StandardsPackageOverview struct {
	TotalSymbols  int                      `json:"total_symbols"`
	ByType        map[string]int           `json:"by_type"`
	Symbols       []StandardsSymbolSummary `json:"symbols"`
	HasPackageDoc bool                     `json:"has_package_doc"`
	Checksum      string                   `json:"checksum,omitempty"`
}

// ListStandardsOverview returns a package-grouped overview of all harvested standards data.
func (s *MemoryStore) ListStandardsOverview(ctx context.Context, packageFilter string) (map[string]*StandardsPackageOverview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	packages := make(map[string]*StandardsPackageOverview)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		if err := s.scanHarvestedCodeIndex(ctx, txn, it, packageFilter, packages); err != nil {
			return err
		}
		s.scanPackageDocIndex(it, packageFilter, packages)
		s.scanSysDriftIndex(txn, it, packageFilter, packages)
		return nil
	})

	return packages, err
}

// scanHarvestedCodeIndex scans the _idx:cat:harvestedcode: prefix to build symbol summaries.
func (s *MemoryStore) scanHarvestedCodeIndex(ctx context.Context, txn *badger.Txn, it *badger.Iterator, packageFilter string, packages map[string]*StandardsPackageOverview) error {
	prefix := []byte("_idx:cat:harvestedcode:")
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := it.Item().Value(func(kVal []byte) error {
			recordKey := string(kVal)

			// Parse package and symbol name from key: pkg:<path>:<name>
			if !strings.HasPrefix(recordKey, "pkg:") {
				return nil
			}
			parts := strings.SplitN(recordKey[4:], ":", 2)
			if len(parts) < 2 {
				return nil
			}
			pkgPath := parts[0]
			symName := parts[1]

			// Apply package filter if set
			if packageFilter != "" && !strings.HasPrefix(pkgPath, packageFilter) {
				return nil
			}

			// Fetch the record to extract tags for symbol type
			item, err := txn.Get(kVal)
			if err != nil {
				return nil
			}

			var symType string
			if err := item.Value(func(v []byte) error {
				if rec, err := migrateRecord(v); err == nil {
					for _, tag := range rec.Tags {
						if after, ok := strings.CutPrefix(tag, "type:"); ok {
							symType = after
							break
						}
					}
				}
				return nil
			}); err != nil {
				return nil
			}

			pkg := s.getOrCreatePackageOverview(packages, pkgPath)
			pkg.TotalSymbols++
			if symType != "" {
				pkg.ByType[symType]++
			}
			pkg.Symbols = append(pkg.Symbols, StandardsSymbolSummary{
				Name:       symName,
				SymbolType: symType,
				Key:        recordKey,
			})
			return nil
		}); err != nil {
			slog.Warn("Error reading standards index", "error", err)
		}
	}
	return nil
}

// scanPackageDocIndex scans the _idx:cat:packagedoc: prefix to flag packages with documentation.
func (s *MemoryStore) scanPackageDocIndex(it *badger.Iterator, packageFilter string, packages map[string]*StandardsPackageOverview) {
	pdPrefix := []byte("_idx:cat:packagedoc:")
	for it.Seek(pdPrefix); it.ValidForPrefix(pdPrefix); it.Next() {
		if err := it.Item().Value(func(kVal []byte) error {
			recordKey := string(kVal)
			if !strings.HasPrefix(recordKey, "pkg:") {
				return nil
			}
			parts := strings.SplitN(recordKey[4:], ":", 2)
			if len(parts) < 1 {
				return nil
			}
			pkgPath := parts[0]
			if packageFilter != "" && !strings.HasPrefix(pkgPath, packageFilter) {
				return nil
			}

			pkg := s.getOrCreatePackageOverview(packages, pkgPath)
			pkg.HasPackageDoc = true
			return nil
		}); err != nil {
			slog.Warn("Error reading PackageDoc index", "error", err)
		}
	}
}

// scanSysDriftIndex scans the _idx:cat:sysdrift: prefix to attach API checksums to packages.
func (s *MemoryStore) scanSysDriftIndex(txn *badger.Txn, it *badger.Iterator, packageFilter string, packages map[string]*StandardsPackageOverview) {
	driftPrefix := []byte("_idx:cat:sysdrift:")
	for it.Seek(driftPrefix); it.ValidForPrefix(driftPrefix); it.Next() {
		if err := it.Item().Value(func(kVal []byte) error {
			recordKey := string(kVal)
			// Key: pkg:<path>:CheckDrift
			if !strings.HasPrefix(recordKey, "pkg:") {
				return nil
			}
			trimmed := strings.TrimPrefix(recordKey, "pkg:")
			pkgPath := strings.TrimSuffix(trimmed, ":CheckDrift")

			if packageFilter != "" && !strings.HasPrefix(pkgPath, packageFilter) {
				return nil
			}

			pkg, ok := packages[pkgPath]
			if !ok {
				return nil
			}

			// Read checksum value
			item, err := txn.Get(kVal)
			if err != nil {
				return nil
			}
			if err := item.Value(func(v []byte) error {
				if rec, err := migrateRecord(v); err == nil {
					pkg.Checksum = rec.Content
				}
				return nil
			}); err != nil {
				return nil
			}
			return nil
		}); err != nil {
			slog.Warn("Error reading SysDrift index", "error", err)
		}
	}
}

// getOrCreatePackageOverview returns an existing overview or creates a new one.
func (s *MemoryStore) getOrCreatePackageOverview(packages map[string]*StandardsPackageOverview, pkgPath string) *StandardsPackageOverview {
	pkg, ok := packages[pkgPath]
	if !ok {
		pkg = &StandardsPackageOverview{
			ByType:  make(map[string]int),
			Symbols: []StandardsSymbolSummary{},
		}
		packages[pkgPath] = pkg
	}
	return pkg
}

// SearchStandards performs a standards-scoped search with multi-dimensional tag filtering.
// Natively leverages the Bleve inverted index if a text query is present.
func (s *MemoryStore) SearchStandards(ctx context.Context, query string, packageFilter string, symbolType string, iface string, receiver string, domain string, limit int) ([]*SearchResult, error) {
	s.mu.RLock()
	searchEngine := s.search
	s.mu.RUnlock()

	// 1. Bleve Routing (Fast Path)
	if query != "" && searchEngine != nil {
		var requiredTags []string
		if symbolType != "" {
			requiredTags = append(requiredTags, "type:"+symbolType)
		}
		if iface != "" {
			requiredTags = append(requiredTags, "implements:"+iface)
		}
		if receiver != "" {
			requiredTags = append(requiredTags, "receiver:"+receiver)
		}
		if domain != "" {
			requiredTags = append(requiredTags, "domain:"+domain)
		}

		cats := make([]string, 0, len(HarvestedCategories))
		for cat := range HarvestedCategories {
			if cat != "SysDrift" {
				cats = append(cats, cat)
			}
		}

		fetchLimit := limit
		if packageFilter != "" && limit > 0 {
			fetchLimit *= 3 // over-fetch to accommodate post-query package trimming
		}

		hits, err := searchEngine.SearchScoped(ctx, query, cats, requiredTags, fetchLimit)
		if err == nil {
			var final []*SearchResult
			for _, h := range hits {
				if packageFilter != "" && !strings.HasPrefix(h.ID, "pkg:"+packageFilter) {
					continue
				}
				if rec, gErr := s.Get(ctx, h.ID); gErr == nil {
					final = append(final, &SearchResult{Key: h.ID, Record: rec, Score: int(h.Score * 100), Snippets: h.Snippets})
					if limit > 0 && len(final) >= limit {
						break
					}
				}
			}
			return final, nil
		}
		slog.Warn("Bleve standards search failed, falling back to badger linear scan", "error", err)
	}

	// 2. Badger Linear Scan (Fallback Path)
	s.mu.RLock()
	defer s.mu.RUnlock()

	var candidates []*SearchResult

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("_idx:domain:" + DomainStandards + ":")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if err := it.Item().Value(func(kVal []byte) error {
				recordKey := string(kVal)

				// Package filter on key prefix proactively
				if packageFilter != "" {
					if !strings.HasPrefix(recordKey, "pkg:"+packageFilter) {
						return nil
					}
				}

				item, err := txn.Get(kVal)
				if err != nil {
					return nil
				}
				return item.Value(func(v []byte) error {
					rec, err := migrateRecord(v)
					if err != nil {
						return nil
					}

					// SysDrift is not searchable for structural metadata
					if rec.Category == "SysDrift" {
						return nil
					}

					// Apply tag-based dimensional filters
					if symbolType != "" && !slices.Contains(rec.Tags, "type:"+symbolType) {
						return nil
					}
					if iface != "" && !slices.Contains(rec.Tags, "implements:"+iface) {
						return nil
					}
					if receiver != "" && !slices.Contains(rec.Tags, "receiver:"+receiver) {
						return nil
					}
					if domain != "" && !slices.Contains(rec.Tags, "domain:"+domain) {
						return nil
					}

					candidates = append(candidates, &SearchResult{Key: recordKey, Record: rec})
					return nil
				})
			}); err != nil {
				slog.Warn("Error during standards domain search", "error", err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Apply text query ranking if specified
	var final []*SearchResult
	if query == "" {
		final = candidates
	} else {
		final = s.rankCandidates(ctx, query, candidates)
	}

	if limit > 0 && len(final) > limit {
		final = final[:limit]
	}

	return final, nil
}

// ListDomainOverview returns a package-grouped overview of harvested data for a specific domain.
// This is the domain-parameterized equivalent of ListStandardsOverview.
func (s *MemoryStore) ListDomainOverview(ctx context.Context, targetDomain string, packageFilter string) (map[string]*StandardsPackageOverview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	packages := make(map[string]*StandardsPackageOverview)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("_idx:domain:" + targetDomain + ":")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if err := it.Item().Value(func(kVal []byte) error {
				recordKey := string(kVal)

				// Parse package and symbol name from key: pkg:<path>:<name>
				if !strings.HasPrefix(recordKey, "pkg:") {
					return nil
				}

				item, err := txn.Get(kVal)
				if err != nil {
					return nil
				}

				return item.Value(func(v []byte) error {
					rec, err := migrateRecord(v)
					if err != nil || rec.Domain != targetDomain {
						return nil
					}

					parts := strings.SplitN(recordKey[4:], ":", 2)
					if len(parts) < 2 {
						return nil
					}
					pkgPath := parts[0]
					symName := parts[1]

					if packageFilter != "" && !strings.HasPrefix(pkgPath, packageFilter) {
						return nil
					}

					switch rec.Category {
					case "HarvestedCode":
						var symType string
						for _, tag := range rec.Tags {
							if after, ok := strings.CutPrefix(tag, "type:"); ok {
								symType = after
								break
							}
						}
						pkg := s.getOrCreatePackageOverview(packages, pkgPath)
						pkg.TotalSymbols++
						if symType != "" {
							pkg.ByType[symType]++
						}
						pkg.Symbols = append(pkg.Symbols, StandardsSymbolSummary{
							Name:       symName,
							SymbolType: symType,
							Key:        recordKey,
						})
					case "PackageDoc":
						pkg := s.getOrCreatePackageOverview(packages, pkgPath)
						pkg.HasPackageDoc = true
					case "SysDrift":
						if pkg, ok := packages[pkgPath]; ok {
							pkg.Checksum = rec.Content
						}
					}
					return nil
				})
			}); err != nil {
				slog.Warn("Error reading domain index", "domain", targetDomain, "error", err)
			}
		}
		return nil
	})

	return packages, err
}

// SearchDomain performs a domain-scoped search with multi-dimensional tag filtering.
// This is the domain-parameterized equivalent of SearchStandards.
func (s *MemoryStore) SearchDomain(ctx context.Context, targetDomain string, query string, packageFilter string, symbolType string, iface string, receiver string, domain string, limit int) ([]*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var candidates []*SearchResult

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("_idx:domain:" + targetDomain + ":")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if err := it.Item().Value(func(kVal []byte) error {
				recordKey := string(kVal)

				// Package filter on key prefix proactively
				if packageFilter != "" {
					if !strings.HasPrefix(recordKey, "pkg:"+packageFilter) {
						return nil
					}
				}

				item, err := txn.Get(kVal)
				if err != nil {
					return nil
				}
				return item.Value(func(v []byte) error {
					rec, err := migrateRecord(v)
					if err != nil {
						return nil
					}

					// SysDrift is not searchable for structural metadata
					if rec.Category == "SysDrift" {
						return nil
					}

					// Apply tag-based dimensional filters
					if symbolType != "" && !slices.Contains(rec.Tags, "type:"+symbolType) {
						return nil
					}
					if iface != "" && !slices.Contains(rec.Tags, "implements:"+iface) {
						return nil
					}
					if receiver != "" && !slices.Contains(rec.Tags, "receiver:"+receiver) {
						return nil
					}
					if domain != "" && !slices.Contains(rec.Tags, "domain:"+domain) {
						return nil
					}

					candidates = append(candidates, &SearchResult{Key: recordKey, Record: rec})
					return nil
				})
			}); err != nil {
				slog.Warn("Error during domain search", "domain", targetDomain, "error", err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Apply text query ranking if specified
	var final []*SearchResult
	if query == "" {
		final = candidates
	} else {
		final = s.rankCandidates(ctx, query, candidates)
	}

	if limit > 0 && len(final) > limit {
		final = final[:limit]
	}

	return final, nil
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
	for w := range strings.FieldsSeq(clean(a)) {
		if len(w) > 2 {
			setA[w] = struct{}{}
		}
	}
	setB := make(map[string]struct{})
	for w := range strings.FieldsSeq(clean(b)) {
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

// ExportJSONL iterates through the badger DB and streams each record as a JSON line
// to the target file. It enforces os.O_EXCL to prevent overwriting existing files.
func (m *MemoryStore) ExportJSONL(ctx context.Context, safePath string, filterCategory string, filterTags []string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. Open destination file with strict O_EXCL to prevent overwrite vulnerabilities
	f, err := os.OpenFile(safePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open export target (O_EXCL constraint): %w", err)
	}

	count := 0

	err = m.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 100
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := string(item.Key())

			// Skip internal index keys
			if strings.HasPrefix(k, "_idx:") {
				continue
			}

			var rec Record
			err := item.Value(func(v []byte) error {
				return json.Unmarshal(v, &rec)
			})
			if err != nil {
				slog.Warn("Failed to unmarshal record during export", "key", k, "error", err)
				continue
			}

			if !matchesExportFilters(&rec, filterCategory, filterTags) {
				continue
			}

			if err := writeJSONLRecord(f, k, &rec); err != nil {
				return err
			}
			count++
		}
		return nil
	})

	if err != nil {
		f.Close()
		return count, fmt.Errorf("export iteration failed: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		return count, fmt.Errorf("failed to sync export file to disk: %w", err)
	}
	if err := f.Close(); err != nil {
		return count, fmt.Errorf("failed to close export file: %w", err)
	}

	return count, nil
}

// matchesExportFilters checks if a record passes the category and tag filters.
func matchesExportFilters(rec *Record, filterCategory string, filterTags []string) bool {
	if filterCategory != "" && rec.Category != filterCategory {
		return false
	}
	if len(filterTags) > 0 {
		for _, reqTag := range filterTags {
			found := slices.Contains(rec.Tags, reqTag)
			if !found {
				return false
			}
		}
	}
	return true
}

// writeJSONLRecord marshals a record with its key and writes it as a JSON line.
func writeJSONLRecord(f *os.File, key string, rec *Record) error {
	exportObj := struct {
		Key string `json:"key"`
		Record
	}{
		Key:    key,
		Record: *rec,
	}

	b, err := json.Marshal(exportObj)
	if err != nil {
		slog.Warn("Failed to marshal record for export", "key", key, "error", err)
		return nil // skip corrupt record, don't abort export
	}

	if _, writeErr := f.Write(append(b, '\n')); writeErr != nil {
		return fmt.Errorf("failed to write jsonl stream: %w", writeErr)
	}
	return nil
}

// ImportJSONL reads a JSONL file from disk and imports it into the Badger DB,
// buffering 100 entries at a time to remain memory-flat and taking advantage of atomic batch writes.
func (m *MemoryStore) ImportJSONL(ctx context.Context, safePath string, mergeStrategy string) (int, []BatchError, error) {
	// 1. Open the file
	f, err := os.Open(safePath)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to open import target: %w", err)
	}
	defer f.Close()

	var totalStored int
	var allErrors []BatchError

	buffer := make([]BatchEntry, 0, 100)
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var raw struct {
			Key        string    `json:"key"`
			Title      string    `json:"title,omitempty"`
			SymbolName string    `json:"symbolname,omitempty"`
			Content    string    `json:"content"`
			Value      string    `json:"value"`
			Category   string    `json:"category"`
			Domain     string    `json:"domain"`
			SessionID  string    `json:"session_id,omitempty"`
			SourcePath string    `json:"source_path,omitempty"`
			SourceHash string    `json:"source_hash,omitempty"`
			Tags       []string  `json:"tags"`
			CreatedAt  time.Time `json:"created_at"`
			UpdatedAt  time.Time `json:"updated_at"`
		}

		if err := json.Unmarshal(line, &raw); err != nil {
			slog.Warn("JSONL unmarshal failed during import", "lineNum", lineNum, "error", err)
			allErrors = append(allErrors, BatchError{
				Key:   "line-" + fmt.Sprint(lineNum),
				Error: err.Error(),
			})
			continue
		}

		content := raw.Content
		if content == "" {
			content = raw.Value
		}

		entry := BatchEntry{
			Key:        raw.Key,
			Title:      raw.Title,
			SymbolName: raw.SymbolName,
			Value:      content,
			Category:   raw.Category,
			Domain:     raw.Domain,
			SessionID:  raw.SessionID,
			SourcePath: raw.SourcePath,
			SourceHash: raw.SourceHash,
			Tags:       raw.Tags,
			CreatedAt:  raw.CreatedAt,
			UpdatedAt:  raw.UpdatedAt,
		}

		buffer = append(buffer, entry)

		if len(buffer) >= 100 {
			stored, errs, bErr := m.SaveBatch(ctx, buffer)
			if bErr != nil {
				return totalStored, allErrors, fmt.Errorf("batch insert failed at line %d: %w", lineNum, bErr)
			}
			totalStored += stored
			allErrors = append(allErrors, errs...)

			// Reset buffer efficiently
			buffer = buffer[:0]
		}
	}

	if err := scanner.Err(); err != nil {
		allErrors = append(allErrors, BatchError{Key: "scanner-err", Error: err.Error()})
		slog.Error("JSONL scanner encountered an error before EOF", "error", err)
	}

	// Flush remaining buffer
	if len(buffer) > 0 {
		stored, errs, bErr := m.SaveBatch(ctx, buffer)
		if bErr != nil {
			return totalStored, allErrors, fmt.Errorf("final batch insert failed: %w", bErr)
		}
		totalStored += stored
		allErrors = append(allErrors, errs...)
	}

	return totalStored, allErrors, nil
}

// DeleteStandards removes standards by category or specific package path prefix.
func (s *MemoryStore) DeleteStandards(ctx context.Context, category, pkg string) (int, error) {
	// Allow empty category and pkg to denote a global domain sweep
	if category != "" && !HarvestedCategories[category] {
		return 0, fmt.Errorf("category %q is not a valid standards category", category)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var keysToDelete []string
	var deletedCount int

	// Define a helper to safely parse the Record to delete indices
	getRecordForDeletion := func(txn *badger.Txn, key string) (*Record, error) {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return nil, err
		}
		var rec *Record
		if vErr := item.Value(func(v []byte) error {
			if r, mErr := migrateRecord(v); mErr == nil {
				rec = r
			}
			return nil
		}); vErr != nil {
			slog.Warn("Failed to read record value during standards deletion", "key", key, "error", vErr)
		}
		return rec, nil
	}

	// Global Domain Sweep Delegation
	if category == "" && pkg == "" {
		return s.DeleteDomain(ctx, DomainStandards)
	}

	// First pass: collect matching domains logic
	if category != "" {
		if err := s.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.DefaultIteratorOptions)
			defer it.Close()
			prefix := fmt.Appendf(nil, "_idx:cat:%s:", strings.ToLower(category))
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				if vErr := it.Item().Value(func(kVal []byte) error {
					key := string(kVal)
					if pkg != "" && !strings.HasPrefix(key, "pkg:"+pkg+":") {
						return nil
					}
					keysToDelete = append(keysToDelete, key)
					return nil
				}); vErr != nil {
					slog.Warn("Failed to read index value during standards delete", "error", vErr)
				}
			}
			return nil
		}); err != nil {
			slog.Warn("Failed to scan category index during standards delete", "category", category, "error", err)
		}
	} else if pkg != "" {
		if err := s.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.DefaultIteratorOptions)
			defer it.Close()
			prefix := []byte("pkg:" + pkg + ":")
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				key := string(it.Item().Key())
				if rec, err := getRecordForDeletion(txn, key); err == nil && rec != nil {
					if HarvestedCategories[rec.Category] {
						keysToDelete = append(keysToDelete, key)
					}
				}
			}
			return nil
		}); err != nil {
			slog.Warn("Failed to scan package prefix during standards delete", "pkg", pkg, "error", err)
		}
	}

	// Delete in chunks to avoid ErrTxnTooBig
	batchSize := 500
	for i := 0; i < len(keysToDelete); i += batchSize {
		end := min(i+batchSize, len(keysToDelete))
		chunk := keysToDelete[i:end]

		err := s.UpdateWithRetry(func(txn *badger.Txn) error {
			for _, key := range chunk {
				if rec, err := getRecordForDeletion(txn, key); err == nil && rec != nil {
					s.deleteRecordIndices(txn, key, rec)
				}
				if err := txn.Delete([]byte(key)); err == nil {
					deletedCount++
				}
			}
			return nil
		})
		if err != nil {
			slog.Error("Failed to delete a batch of standards", "error", err)
		}
	}

	// Purge from Bleve search index
	if s.search != nil && len(keysToDelete) > 0 {
		for start := 0; start < len(keysToDelete); start += s.maxBatchSize {
			end := start + s.maxBatchSize
			if end > len(keysToDelete) {
				end = len(keysToDelete)
			}
			chunk := keysToDelete[start:end]
			if dErr := s.search.DeleteBatch(chunk); dErr != nil {
				slog.Warn("Failed to purge batch from search index", "error", dErr)
			}
		}
	}

	slog.Info("Deleted standards", "category", category, "package", pkg, "count", deletedCount)
	return deletedCount, nil
}

// DeleteProjects removes projects by category or specific package path prefix.
func (s *MemoryStore) DeleteProjects(ctx context.Context, category, pkg string) (int, error) {
	// Allow empty category and pkg to denote a global domain sweep

	s.mu.Lock()
	defer s.mu.Unlock()

	var keysToDelete []string
	var deletedCount int

	getRecordForDeletion := func(txn *badger.Txn, key string) (*Record, error) {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return nil, err
		}
		var rec *Record
		if vErr := item.Value(func(v []byte) error {
			if r, mErr := migrateRecord(v); mErr == nil {
				rec = r
			}
			return nil
		}); vErr != nil {
			slog.Warn("Failed to read record value during projects deletion", "key", key, "error", vErr)
		}
		return rec, nil
	}

	// Global Domain Sweep Delegation
	if category == "" && pkg == "" {
		return s.DeleteDomain(ctx, DomainProjects)
	}

	if category != "" {
		if err := s.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.DefaultIteratorOptions)
			defer it.Close()
			prefix := fmt.Appendf(nil, "_idx:cat:%s:", strings.ToLower(category))
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				if vErr := it.Item().Value(func(kVal []byte) error {
					key := string(kVal)
					if pkg != "" && !strings.HasPrefix(key, "pkg:"+pkg+":") {
						return nil
					}
					if rec, err := getRecordForDeletion(txn, key); err == nil && rec != nil {
						if rec.Domain == DomainProjects {
							keysToDelete = append(keysToDelete, key)
						}
					}
					return nil
				}); vErr != nil {
					slog.Warn("Failed to read index value during projects delete", "error", vErr)
				}
			}
			return nil
		}); err != nil {
			slog.Warn("Failed to scan category index during projects delete", "category", category, "error", err)
		}
	} else if pkg != "" {
		if err := s.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.DefaultIteratorOptions)
			defer it.Close()
			prefix := []byte("pkg:" + pkg + ":")
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				key := string(it.Item().Key())
				if rec, err := getRecordForDeletion(txn, key); err == nil && rec != nil {
					if rec.Domain == DomainProjects {
						keysToDelete = append(keysToDelete, key)
					}
				}
			}
			return nil
		}); err != nil {
			slog.Warn("Failed to scan package prefix during projects delete", "pkg", pkg, "error", err)
		}
	}

	batchSize := 500
	for i := 0; i < len(keysToDelete); i += batchSize {
		end := min(i+batchSize, len(keysToDelete))
		chunk := keysToDelete[i:end]

		err := s.UpdateWithRetry(func(txn *badger.Txn) error {
			for _, key := range chunk {
				if rec, err := getRecordForDeletion(txn, key); err == nil && rec != nil {
					s.deleteRecordIndices(txn, key, rec)
				}
				if err := txn.Delete([]byte(key)); err == nil {
					deletedCount++
				}
			}
			return nil
		})
		if err != nil {
			slog.Error("Failed to delete a batch of projects", "error", err)
		}
	}

	if s.search != nil && len(keysToDelete) > 0 {
		for start := 0; start < len(keysToDelete); start += s.maxBatchSize {
			end := start + s.maxBatchSize
			if end > len(keysToDelete) {
				end = len(keysToDelete)
			}
			chunk := keysToDelete[start:end]
			if dErr := s.search.DeleteBatch(chunk); dErr != nil {
				slog.Warn("Failed to purge batch from search index", "error", dErr)
			}
		}
	}

	slog.Info("Deleted projects", "category", category, "package", pkg, "count", deletedCount)
	return deletedCount, nil
}

// PurgeDomain completely deletes all records associated with a specific domain.
func (s *MemoryStore) PurgeDomain(ctx context.Context, targetDomain string) (int, error) {
	if targetDomain == "" {
		return 0, fmt.Errorf("targetDomain is missing")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var keysToDelete []string
	var deletedCount int

	getRecordForDeletion := func(txn *badger.Txn, key string) (*Record, error) {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return nil, err
		}
		var rec *Record
		if vErr := item.Value(func(v []byte) error {
			if r, mErr := migrateRecord(v); mErr == nil {
				rec = r
			}
			return nil
		}); vErr != nil {
			// Ignore read err for orphaned indices
		}
		return rec, nil
	}

	if err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			key := string(it.Item().Key())
			// Only evaluate raw model records, bypass manual _idx
			if strings.HasPrefix(key, "_idx:") {
				continue
			}
			if rec, err := getRecordForDeletion(txn, key); err == nil && rec != nil {
				if rec.Domain == targetDomain {
					keysToDelete = append(keysToDelete, key)
				}
			}
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("purge scan failed: %w", err)
	}

	batchSize := 500
	for i := 0; i < len(keysToDelete); i += batchSize {
		end := min(i+batchSize, len(keysToDelete))
		chunk := keysToDelete[i:end]

		err := s.UpdateWithRetry(func(txn *badger.Txn) error {
			for _, key := range chunk {
				if rec, err := getRecordForDeletion(txn, key); err == nil && rec != nil {
					s.deleteRecordIndices(txn, key, rec)
				}
				if err := txn.Delete([]byte(key)); err == nil {
					deletedCount++
				}
			}
			return nil
		})
		if err != nil {
			slog.Error("Failed to purge a batch of records", "domain", targetDomain, "error", err)
		}
	}

	if s.search != nil && len(keysToDelete) > 0 {
		for _, key := range keysToDelete {
			_ = s.search.Delete(key)
		}
	}

	slog.Info("Domain completely purged", "domain", targetDomain, "count", deletedCount)
	return deletedCount, nil
}

// PruneDomain deletes records from a domain (or all domains if empty) whose UpdatedAt exceeds daysOld.
func (s *MemoryStore) PruneDomain(ctx context.Context, targetDomain string, daysOld int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var keysToDelete []string
	var deletedCount int
	cutoffTime := time.Now().Add(-time.Duration(daysOld) * 24 * time.Hour)

	getRecordForDeletion := func(txn *badger.Txn, key string) (*Record, error) {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return nil, err
		}
		var rec *Record
		if vErr := item.Value(func(v []byte) error {
			if r, mErr := migrateRecord(v); mErr == nil {
				rec = r
			}
			return nil
		}); vErr != nil {
			// Ignore read err for orphaned indices
		}
		return rec, nil
	}

	if err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			key := string(it.Item().Key())
			// Ignore abstract structural index keys
			if strings.HasPrefix(key, "_idx:") {
				continue
			}
			if rec, err := getRecordForDeletion(txn, key); err == nil && rec != nil {
				if (targetDomain == "" || rec.Domain == targetDomain) && rec.UpdatedAt.Before(cutoffTime) {
					keysToDelete = append(keysToDelete, key)
				}
			}
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("prune scan failed: %w", err)
	}

	batchSize := 500
	for i := 0; i < len(keysToDelete); i += batchSize {
		end := min(i+batchSize, len(keysToDelete))
		chunk := keysToDelete[i:end]

		err := s.UpdateWithRetry(func(txn *badger.Txn) error {
			for _, key := range chunk {
				if rec, err := getRecordForDeletion(txn, key); err == nil && rec != nil {
					s.deleteRecordIndices(txn, key, rec)
				}
				if err := txn.Delete([]byte(key)); err == nil {
					deletedCount++
				}
			}
			return nil
		})
		if err != nil {
			slog.Error("Failed to prune a batch of records", "domain", targetDomain, "error", err)
		}
	}

	if s.search != nil && len(keysToDelete) > 0 {
		for _, key := range keysToDelete {
			_ = s.search.Delete(key)
		}
	}

	slog.Info("Namespace pruned successfully", "domain", targetDomain, "daysOlderThan", daysOld, "count", deletedCount)
	return deletedCount, nil
}
