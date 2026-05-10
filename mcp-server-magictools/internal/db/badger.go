package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-server-magictools/internal/telemetry"
	"mcp-server-magictools/internal/util"
	"mcp-server-magictools/internal/vector"
)

type CacheMetrics struct {
	Hits      uint64
	Misses    uint64
	Entries   int
	Tools     int
	Intel     int
	BleveDocs uint64
}

// SearchDomain defines the logical visibility scope for a tool search request.
type SearchDomain string

const (
	// DomainUserLand: Mask brainstorm and go-refactor tools (Default for align_tools)
	DomainUserLand SearchDomain = "user_land"
	// DomainPipelineOrchestration: Search only brainstorm, go-refactor, and magictools synthesizers
	DomainPipelineOrchestration SearchDomain = "pipeline_orchestration"
	// DomainSystem: Search all tools without sharding (Diagnostics/Maintenance)
	DomainSystem SearchDomain = "system"
)

// Store wraps BadgerDB and Bleve index
type Store struct {
	DB                *badger.DB
	Index             *SearchIndex
	Path              string // 🛡️ BASTION ROOT: The absolute path to the database directory
	PidPath           string
	Cache             *RegistryCache
	PendingHydrations atomic.Int64 // 🛡️ DIRTY FLAG: Tracks pending hydration count for optimistic skip
	toolsCount        atomic.Int64
	intelCount        atomic.Int64
	SynergySuccess    sync.Map       // Tracks RRF synergy strengths
	SynergyPenalty    sync.Map       // Tracks RRF synergy rejections
	closing           chan struct{}  // 🛡️ LIFECYCLE: Signaled on Close() to abort background goroutines
	bgWg              sync.WaitGroup // 🛡️ LIFECYCLE: Blocks Close() until background goroutines halt
}

// ToolMetrics stores dynamic intent reliability scoring for search rescoring
type ToolMetrics struct {
	ProxyReliability     float64 `json:"proxy_reliability"`
	TotalSuccessfulCalls int     `json:"total_successful_calls"`
	TotalCalls           int     `json:"total_calls"`
	FailureRate          float64 `json:"failure_rate"`
	AvgLatencyMs         int64   `json:"avg_latency_ms"`
	LastErrorClass       string  `json:"last_error_class,omitempty"`
}

// ToolRecord is undocumented but satisfies standard structural requirements.
type ToolRecord struct {
	URN         string         `json:"urn"`
	Name        string         `json:"name"`
	Server      string         `json:"server"`
	Description string         `json:"description"` // Full description
	InputSchema map[string]any `json:"input_schema"`
	LiteSummary string         `json:"lite_summary"`
	Category    string         `json:"category"`
	DependsOn   []string       `json:"depends_on"` // Required URNs for topological DAG sorting
	Requires    []string       `json:"requires,omitempty"`
	Triggers    []string       `json:"triggers,omitempty"`

	// 🛡️ PIPELINE TAXONOMY: Role and Phase classification for compose_pipeline DAG intelligence.
	// Role: ANALYZER, MUTATOR, CRITIC, SYNTHESIZER, DIAGNOSTIC
	// Phase: 1=DISCOVERY, 2=ANALYSIS, 3=PROPOSAL, 4=ADVERSARIAL, 5=SYNTHESIS
	Role           string `json:"role,omitempty"`
	Phase          int    `json:"phase,omitempty"`
	InputContract  string `json:"input_contract,omitempty"`
	OutputContract string `json:"output_contract,omitempty"`

	SchemaHash   string         `json:"schema_hash"`
	LastSyncedAt int64          `json:"last_synced_at"`
	UsageCount   int64          `json:"usage_count"`
	LastUsedAt   int64          `json:"last_used_at"`
	TimeoutSecs  int            `json:"timeout_secs,omitempty"`
	IsNative     bool           `json:"is_native,omitempty"`
	Intent         string         `json:"intent,omitempty"`
	ZeroValues     map[string]any `json:"zero_values,omitempty"` // Pre-computed schema defaults and fast auto-coercion fallbacks natively
	ParameterNames []string       `json:"parameter_names,omitempty"` // Materialized from InputSchema.properties keys for Bleve keyword + HNSW embedding
	Metrics        ToolMetrics    `json:"-"`

	// Intelligence
	AnalysisStatus   string   `json:"-"` // pending, hydrated, failed
	SyntheticIntents []string `json:"-"`
	LexicalTokens    []string `json:"-"`
	NegativeTriggers []string `json:"-"`

	// Diagnostics (Transient)
	ConfidenceScore        float64 `json:"-"`
	HighlightedDescription string  `json:"-"`
}

// ToolIntelligence structurally defines the persisted semantic properties.
// It is intentionally split from ToolRecord to prevent aggressive orchestrator wiping (tools/list sync)
// from terminating previously learned LLM intents and utilization proxies.
type ToolIntelligence struct {
	AnalysisStatus   string      `json:"analysis_status,omitempty"` // pending, hydrated, failed
	SchemaHash       string      `json:"schema_hash,omitempty"`
	SyntheticIntents []string    `json:"synthetic_intents,omitempty"`
	LexicalTokens    []string    `json:"lexical_tokens,omitempty"`
	NegativeTriggers []string    `json:"negative_triggers,omitempty"`
	Metrics          ToolMetrics `json:"metrics"`
}

// countKeys performs a fast prefix-based scan counting total BadgerDB keys, exclusively for cold-boot seeding
func (s *Store) countKeys(prefix string) (int, error) {
	count := 0
	err := s.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Optimize explicitly for counting keys
		it := txn.NewIterator(opts)
		defer it.Close()
		p := []byte(prefix)
		for it.Seek(p); it.ValidForPrefix(p); it.Next() {
			count++
		}
		return nil
	})
	return count, err
}

// GetMetrics returns a unified snapshot of Cache, Store, and Intelligence sizes
func (s *Store) GetMetrics() CacheMetrics {
	var metrics CacheMetrics
	if s == nil {
		return metrics
	}
	if s.Cache != nil {
		h, m, e := s.Cache.GetMetrics()
		metrics.Hits = h
		metrics.Misses = m
		metrics.Entries = e
	}
	metrics.Tools = int(s.toolsCount.Load())
	metrics.Intel = int(s.intelCount.Load())

	if s.Index != nil {
		if count, err := s.Index.DocCount(); err == nil {
			metrics.BleveDocs = count
		}
	}
	return metrics
}

// RecordSynergy synchronously updates RAM models and dispatches an async BadegerDB transaction safely.
func (s *Store) RecordSynergy(hash string, success bool) {
	if s == nil || s.DB == nil {
		return
	}
	succKey := []byte("synergy:success:" + hash)
	penKey := []byte("synergy:penalty:" + hash)

	succVal, _ := s.SynergySuccess.LoadOrStore(hash, &atomic.Int64{})
	penVal, _ := s.SynergyPenalty.LoadOrStore(hash, &atomic.Int64{})
	succCounter := succVal.(*atomic.Int64)
	penCounter := penVal.(*atomic.Int64)

	currSucc := succCounter.Load()
	currPen := penCounter.Load()

	// 🚀 CHRONOLOGICAL ENTROPY DECAY CEILING GUARD
	decayTriggered := false
	if currSucc+currPen > 500 {
		currSucc = currSucc / 2
		currPen = currPen / 2
		succCounter.Store(currSucc)
		penCounter.Store(currPen)
		decayTriggered = true
	}

	var newSucc, newPen int64
	if success {
		newSucc = succCounter.Add(1)
		newPen = penCounter.Load()
	} else {
		newPen = penCounter.Add(1)
		newSucc = succCounter.Load()
	}

	// Async Disk Flush natively managing combined states if geometric bounds were truncated organically
	go func(sK, pK []byte, sV, pV int64, updateBoth bool) {
		err := s.DB.Update(func(txn *badger.Txn) error {
			if updateBoth {
				if err := txn.Set(sK, []byte(strconv.FormatInt(sV, 10))); err != nil {
					return err
				}
				return txn.Set(pK, []byte(strconv.FormatInt(pV, 10)))
			}
			if success {
				return txn.Set(sK, []byte(strconv.FormatInt(sV, 10)))
			}
			return txn.Set(pK, []byte(strconv.FormatInt(pV, 10)))
		})
		if err != nil {
			slog.Error("database: failed to persist RRF synergy weight", "hash", hash, "error", err)
		}
	}(succKey, penKey, newSucc, newPen, decayTriggered)
}

// GetSynergy returns the active heuristic trust models for the given Hash transition in O(1) latency.
func (s *Store) GetSynergy(hash string) (successes int64, penalties int64) {
	if s == nil {
		return 0, 0
	}
	if v, ok := s.SynergySuccess.Load(hash); ok {
		successes = v.(*atomic.Int64).Load()
	}
	if v, ok := s.SynergyPenalty.Load(hash); ok {
		penalties = v.(*atomic.Int64).Load()
	}
	return successes, penalties
}

// seedSynergyWeights parses historical synergy on boot.
func (s *Store) seedSynergyWeights() {
	s.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true

		// Load Successes
		it := txn.NewIterator(opts)
		defer it.Close()
		prefixS := []byte("synergy:success:")
		for it.Seek(prefixS); it.ValidForPrefix(prefixS); it.Next() {
			item := it.Item()
			hash := strings.TrimPrefix(string(item.Key()), string(prefixS))
			item.Value(func(v []byte) error {
				if val, err := strconv.ParseInt(string(v), 10, 64); err == nil {
					counter := &atomic.Int64{}
					counter.Store(val)
					s.SynergySuccess.Store(hash, counter)
				}
				return nil
			})
		}

		// Load Penalties
		it2 := txn.NewIterator(opts)
		defer it2.Close()
		prefixP := []byte("synergy:penalty:")
		for it2.Seek(prefixP); it2.ValidForPrefix(prefixP); it2.Next() {
			item := it2.Item()
			hash := strings.TrimPrefix(string(item.Key()), string(prefixP))
			item.Value(func(v []byte) error {
				if val, err := strconv.ParseInt(string(v), 10, 64); err == nil {
					counter := &atomic.Int64{}
					counter.Store(val)
					s.SynergyPenalty.Store(hash, counter)
				}
				return nil
			})
		}
		return nil
	})
}

// StartBackgroundGC triggers Badger's value log garbage collection in a loop.
// This is critical for bastion environments to keep the disk footprint small.
func (s *Store) StartBackgroundGC(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Run GC if we can reclaim at least 50% of the space in a log file.
			err := s.DB.RunValueLogGC(0.5)
			if err == nil {
				slog.Info("database: value log garbage collection successful")
			} else if err != badger.ErrNoRewrite {
				slog.Error("database: value log garbage collection failed", "error", err)
			}
		}
	}
}

// NewStore initializes Badger and Bleve with lock cleanup logic
func NewStore(path string, limitOpts ...int) (*Store, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	if err := cleanupStaleProcess(path); err != nil {
		slog.Warn("Cleanup of stale process failed", "error", err)
	}

	if err := AcquireLock(path); err != nil {
		return nil, err
	}

	// 1. Initialize Primary Storage (Badger)
	opts := badger.DefaultOptions(path).
		WithLogger(nil).
		WithSyncWrites(false).                             // Changed to false to avoid massive sync blocks during JSON ingest
		WithCompression(options.ZSTD).                     // Force ZSTD for the VLOG to stop the 2GB bloat
		WithValueThreshold(1 << 20).                       // 1MB — keeps values in LSM tree
		WithValueLogFileSize(16 << 20).                    // 16MB — safety net rotation
		WithValueLogMaxEntries(100).                       // Aggressive rotation
		WithNumVersionsToKeep(1).                          // Keep only latest version
		WithIndexCacheSize(16 << 20).                      // 16MB
		WithBlockCacheSize(32 << 20).                      // 32MB
		WithMemTableSize(8 << 20).                         // 8MB
		WithNumMemtables(2).                               // 2
		WithNumLevelZeroTables(2).                         // 2
		WithNumLevelZeroTablesStall(4).                    // 4
		WithCompactL0OnClose(false).                       // 🛡️ F4: Avoid massive IO stall during Bastion shutdown
		WithChecksumVerificationMode(options.OnTableRead). // 🛡️ F5: Verify SST checksums on open — detect silent corruption
		WithNumGoroutines(4)                               // 🛡️ Constrain CPU threads to 2 CPUs limit

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}

	// 2. Initialize Search Index (Bleve)
	index, err := NewSearchIndex(path)
	if err != nil {
		if closeErr := db.Close(); closeErr != nil {
			slog.Error("Failed to close database after search index failure", "error", closeErr)
		}
		return nil, fmt.Errorf("failed to open search index: %w", err)
	}

	pidPath := filepath.Join(path, "server.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		slog.Warn("Failed to write PID file", "error", err)
	}

	s := &Store{
		DB:      db,
		Index:   index,
		Path:    path,
		PidPath: pidPath,
		Cache:   NewRegistryCache(limitOpts...),
		closing: make(chan struct{}),
	}

	// 3. Reconcile drift or empty
	docCount, dErr := index.DocCount()
	keyCount, kErr := s.countKeys("tool:")
	drifting := false
	if dErr == nil && kErr == nil && docCount != uint64(keyCount) {
		drifting = true
		slog.Warn("Index drift detected", "badger_keys", keyCount, "bleve_docs", docCount)
	}

	// 🛡️ COLD BOOT ATOMIC SEEDING
	s.toolsCount.Store(int64(keyCount))
	iCount, _ := s.countKeys("intel:")
	s.intelCount.Store(int64(iCount))
	s.seedSynergyWeights()

	// Lazy-reindex if the search index is empty or drifted (Backgrounded to avoid IDE handshake timeouts)
	if index.IsEmpty() || drifting {
		s.bgWg.Add(1)
		go func(c context.Context) {
			defer s.bgWg.Done()
			// 🛡️ LIFECYCLE GUARD: Abort if Store.Close() was called before this goroutine starts.
			select {
			case <-s.closing:
				return
			default:
			}
			if drifting {
				slog.Info("Search index has drifted, performing full re-index in background...")
			} else {
				slog.Info("Search index is empty, performing lazy re-index in background...")
			}
			if err := s.ReindexAllTools(); err != nil {
				slog.Error("Failed to re-index all tools", "error", err)
			} else {
				slog.Info("Lazy re-indexing complete.")
			}
		}(context.Background())
	}

	return s, nil
}

// Close closes the database and index
func (s *Store) Close() error {
	// 🛡️ LIFECYCLE: Signal background goroutines to abort before closing DB.
	close(s.closing)
	s.bgWg.Wait() // Guaranteed shutdown before DB Reference drop

	if err := os.Remove(s.PidPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("Failed to remove PID file on close", "path", s.PidPath, "error", err)
	}
	if dbLockFile != nil {
		if err := releaseOSLock(dbLockFile); err != nil {
			slog.Warn("Failed to release database lock", "error", err)
		}
		if err := dbLockFile.Close(); err != nil {
			slog.Warn("Failed to close database lock file", "error", err)
		}
	}
	if err := s.Index.Close(); err != nil {
		slog.Error("Failed to close search index", "error", err)
	}
	return s.DB.Close()
}

// UpdateWithRetry wraps badger.Update with exponential backoff for Transaction Conflicts.
func (s *Store) UpdateWithRetry(fn func(txn *badger.Txn) error) error {
	maxRetries := 5
	backoff := 10 * time.Millisecond

	var err error
	for range maxRetries {
		err = s.DB.Update(fn)
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

// BatchSaveTools saves multiple tools and their schemas in a single transaction.
func (s *Store) BatchSaveTools(records []*ToolRecord, schemas map[string]map[string]any) error {
	var totalNewTools int64

	err := s.UpdateWithRetry(func(txn *badger.Txn) error {
		totalNewTools = 0 // Reset counter for retry safety
		// 1. Save all schemas
		for hash, schema := range schemas {
			data, err := json.Marshal(schema)
			if err != nil {
				continue // Skip bad schemas
			}
			compressed, err := util.Compress(data)
			if err != nil {
				continue
			}
			if err := txn.Set([]byte("schema:"+hash), compressed); err != nil {
				return err
			}
		}

		// 2. Save all tool records
		for _, record := range records {
			data, err := json.Marshal(record)
			if err != nil {
				continue
			}
			isNew := false
			if _, gErr := txn.Get([]byte("tool:" + record.URN)); gErr == badger.ErrKeyNotFound {
				isNew = true
			}
			if err := txn.Set([]byte("tool:"+record.URN), data); err != nil {
				return err
			}
			if isNew {
				totalNewTools++
			}
			// Update category index
			catKey := []byte("cat:" + record.Category + ":" + record.URN)
			if err := txn.Set(catKey, []byte(record.URN)); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	if totalNewTools > 0 {
		s.toolsCount.Add(totalNewTools)
	}

	// 3. Post-transaction updates (Search Index and Micro-Cache)
	s.Cache.SetCategories(nil)
	for hash, schema := range schemas {
		s.Cache.Set("schema:"+hash, schema, 2*time.Hour)
	}

	// 🛡️ BATCH INTELLIGENCE HYDRATION
	intelMap := make(map[string]*ToolIntelligence, len(records))
	_ = s.DB.View(func(txn *badger.Txn) error {
		for _, record := range records {
			if item, err := txn.Get([]byte("intel:" + record.URN)); err == nil {
				_ = item.Value(func(val []byte) error {
					var intel ToolIntelligence
					if json.Unmarshal(val, &intel) == nil {
						intelMap[record.URN] = &intel
					}
					return nil
				})
			}
		}
		return nil
	})

	var bleveBatch []BleveToolDocument
	for _, record := range records {
		if intel, exists := intelMap[record.URN]; exists {
			record.AnalysisStatus = intel.AnalysisStatus
			record.SyntheticIntents = intel.SyntheticIntents
			record.LexicalTokens = intel.LexicalTokens
			record.NegativeTriggers = intel.NegativeTriggers
			record.Metrics = intel.Metrics
		}
		bleveBatch = append(bleveBatch, ToBleveDoc(record))
		s.Cache.Set("tool:"+record.URN, record, 2*time.Hour)
	}

	if len(bleveBatch) > 0 {
		if err := s.Index.IndexBatch(bleveBatch); err != nil {
			slog.Warn("Failed to update search index for tools in batch", "error", err)
		}
	}

	return nil
}

// SaveTool persists a tool record and triggers a 1:1 Bleve index sync.
func (s *Store) SaveTool(record *ToolRecord) error {
	s.Cache.Delete(record.Server)
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	err = s.DB.Update(func(txn *badger.Txn) error {
		// Save main tool record
		isNew := false
		if _, gErr := txn.Get([]byte("tool:" + record.URN)); gErr == badger.ErrKeyNotFound {
			isNew = true
		}
		err := txn.Set([]byte("tool:"+record.URN), data)
		if err != nil {
			return err
		}
		if isNew {
			s.toolsCount.Add(1)
		}

		// Update category index
		catKey := []byte("cat:" + record.Category + ":" + record.URN)
		return txn.Set(catKey, []byte(record.URN))
	})
	if err != nil {
		return err
	}

	// 🛡️ DYNAMIC OVERLAY: Aggregate semantic parameters if they exist for this schema before indexing
	if intel, err := s.GetIntelligence(record.URN); err == nil && intel != nil {
		record.AnalysisStatus = intel.AnalysisStatus
		record.SyntheticIntents = intel.SyntheticIntents
		record.LexicalTokens = intel.LexicalTokens
		record.NegativeTriggers = intel.NegativeTriggers
		record.Metrics = intel.Metrics
	}

	// Update search index
	if err := s.Index.IndexRecord(ToBleveDoc(record)); err != nil {
		slog.Warn("Failed to update search index for tool", "urn", record.URN, "error", err)
	}

	// CACHE update: Lower I/O for subsequent Definition/Schema lookups.
	s.Cache.Set("tool:"+record.URN, record, 2*time.Hour)
	// Invalidate category cache (new category could have been added)
	s.Cache.SetCategories(nil)
	return nil
}

// GetIntelligence retrieves the LLM-mapped traits for a particular ToolRecord.
// This data resides under the intel:<urn> namespace permanently.
func (s *Store) GetIntelligence(urn string) (*ToolIntelligence, error) {
	var intel ToolIntelligence
	err := s.DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("intel:" + urn))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &intel)
		})
	})
	if err != nil {
		return nil, err
	}
	return &intel, nil
}

// SaveIntelligence persists semantic knowledge under the intel:<urn> namespace.
// Triggering cache eviction and search re-indexing structurally coordinates orchestrator awareness.
func (s *Store) SaveIntelligence(urn string, intel *ToolIntelligence) error {
	data, err := json.Marshal(intel)
	if err != nil {
		return err
	}
	err = s.DB.Update(func(txn *badger.Txn) error {
		isNew := false
		if _, gErr := txn.Get([]byte("intel:" + urn)); gErr == badger.ErrKeyNotFound {
			isNew = true
		}
		e := txn.Set([]byte("intel:"+urn), data)
		if e == nil && isNew {
			s.intelCount.Add(1)
		}
		return e
	})

	if err != nil {
		return err
	}

	// Evict stale tool schema and dynamically re-index
	s.Cache.Delete("tool:" + urn)
	go func() {
		mergedRecord, err := s.GetTool(urn)
		if err == nil && mergedRecord != nil {
			if err := s.Index.IndexRecord(ToBleveDoc(mergedRecord)); err != nil {
				slog.Warn("Failed to update search index for intelligence overlay", "urn", urn, "error", err)
			}
		}
	}()
	return nil
}

var lastToolSync atomic.Int64

// UpdateToolMetrics modifies the dynamic index weight based on proxy call execution results
func (s *Store) UpdateToolMetrics(urn string, success bool, confidence float64) error {
	var intel ToolIntelligence
	var err error

	err = s.UpdateWithRetry(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("intel:" + urn))
		if err == badger.ErrKeyNotFound {
			// Initialize new if empty
			intel = ToolIntelligence{}
		} else if err != nil {
			return err
		} else {
			err = item.Value(func(val []byte) error {
				return json.Unmarshal(val, &intel)
			})
			if err != nil {
				return err
			}
		}

		if intel.Metrics.ProxyReliability == 0 {
			intel.Metrics.ProxyReliability = 1.0 // default W_base initialization
		}

		delta := 0.005
		intel.Metrics.TotalCalls++
		if success {
			intel.Metrics.ProxyReliability += delta
			intel.Metrics.TotalSuccessfulCalls++
		} else {
			if intel.Metrics.TotalSuccessfulCalls >= 50 {
				intel.Metrics.ProxyReliability -= (delta / 2.0)
			} else {
				intel.Metrics.ProxyReliability -= delta
			}
		}

		// Recompute failure rate from absolute counters.
		if intel.Metrics.TotalCalls > 0 {
			intel.Metrics.FailureRate = 1.0 - float64(intel.Metrics.TotalSuccessfulCalls)/float64(intel.Metrics.TotalCalls)
		}

		if intel.Metrics.ProxyReliability < 0.5 {
			intel.Metrics.ProxyReliability = 0.5
		}
		if intel.Metrics.ProxyReliability > 2.0 {
			intel.Metrics.ProxyReliability = 2.0
		}

		data, err := json.Marshal(intel)
		if err != nil {
			return err
		}
		return txn.Set([]byte("intel:"+urn), data)
	})

	if err != nil {
		return fmt.Errorf("failed to update metrics: %w", err)
	}

	// Async reindex & disk sync
	go func() {
		// Asynchronously commit OS dirty pages to disk every 2 seconds to keep the CLI ReadOnly dashboard crash-free
		now := time.Now().UnixMilli()
		if last := lastToolSync.Load(); now-last > 2000 {
			if lastToolSync.CompareAndSwap(last, now) {
				_ = s.DB.Sync()
			}
		}

		// Evict the stale struct from cache so GetTool triggers a full native KV merge
		s.Cache.Delete("tool:" + urn)
		// DYNAMIC OVERLAY: Re-fetch entire tool with the newly updated `intel` metrics!
		mergedRecord, err := s.GetTool(urn)
		if err == nil && mergedRecord != nil {
			if err := s.Index.IndexRecord(ToBleveDoc(mergedRecord)); err != nil {
				slog.Warn("Failed to update search index for tool metrics", "urn", urn, "error", err)
			}
		}
	}()

	return nil
}

// IncrementToolCalls updates the rolling average latency for a tool after a proxy call.
// Uses Welford's online algorithm: new_avg = old_avg + (sample - old_avg) / n.
func (s *Store) IncrementToolCalls(urn string, latencyMs int64) {
	if s == nil || s.DB == nil {
		return
	}

	_ = s.UpdateWithRetry(func(txn *badger.Txn) error {
		var intel ToolIntelligence
		item, err := txn.Get([]byte("intel:" + urn))
		if err == badger.ErrKeyNotFound {
			return nil // No intel record — skip
		} else if err != nil {
			return err
		}

		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &intel)
		}); err != nil {
			return err
		}

		// Welford's rolling average for latency.
		if intel.Metrics.TotalCalls > 0 {
			intel.Metrics.AvgLatencyMs += (latencyMs - intel.Metrics.AvgLatencyMs) / int64(intel.Metrics.TotalCalls)
		} else {
			intel.Metrics.AvgLatencyMs = latencyMs
		}

		data, err := json.Marshal(intel)
		if err != nil {
			return err
		}
		return txn.Set([]byte("intel:"+urn), data)
	})
}

// RecordToolError records the most recent error class on a tool's intelligence record.
func (s *Store) RecordToolError(urn string, errorClass string) {
	if s == nil || s.DB == nil {
		return
	}

	_ = s.UpdateWithRetry(func(txn *badger.Txn) error {
		var intel ToolIntelligence
		item, err := txn.Get([]byte("intel:" + urn))
		if err == badger.ErrKeyNotFound {
			return nil // No intel record — skip
		} else if err != nil {
			return err
		}

		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &intel)
		}); err != nil {
			return err
		}

		intel.Metrics.LastErrorClass = errorClass

		data, err := json.Marshal(intel)
		if err != nil {
			return err
		}
		return txn.Set([]byte("intel:"+urn), data)
	})
}

// SaveSchema persists a deduplicated tool schema
func (s *Store) SaveSchema(hash string, schema map[string]any) error {
	data, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	compressed, err := util.Compress(data)
	if err != nil {
		return fmt.Errorf("failed to compress schema: %w", err)
	}
	err = s.DB.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("schema:"+hash), compressed)
	})
	if err == nil {
		s.Cache.Set("schema:"+hash, schema, 2*time.Hour)
	}
	return err
}

// GetSchema retrieves a tool schema by hash, using the Micro-Cache to avoid Badger/ZSTD overhead.
func (s *Store) GetSchema(hash string) (map[string]any, error) {
	if hash == "" {
		return nil, badger.ErrKeyNotFound
	}
	// 1. Cache Check
	if val, ok := s.Cache.Get("schema:" + hash); ok {
		if m, ok := val.(map[string]any); ok {
			return m, nil
		}
	}

	var m map[string]any
	err := s.DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("schema:" + hash))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			decompressed, err := util.Decompress(val)
			if err != nil {
				return err
			}
			return json.Unmarshal(decompressed, &m)
		})
	})
	if err == nil {
		s.Cache.Set("schema:"+hash, m, 2*time.Hour)
	}
	return m, err
}

// GetTool retrieves a tool record by URN, prioritizing the Micro-Cache to lower read I/O.
func (s *Store) GetTool(urn string) (*ToolRecord, error) {
	// 1. Cache Check (Bastion's best friend)
	if val, ok := s.Cache.Get("tool:" + urn); ok {
		if record, ok := val.(*ToolRecord); ok {
			return record, nil
		}
	}

	var record ToolRecord
	err := s.DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("tool:" + urn))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &record)
		})
	})
	if err != nil {
		return nil, err
	}

	// 🛡️ DYNAMIC OVERLAY: Aggregate semantic parameters if they exist for this schema
	if intel, err := s.GetIntelligence(urn); err == nil && intel != nil {
		record.AnalysisStatus = intel.AnalysisStatus
		record.SyntheticIntents = intel.SyntheticIntents
		record.LexicalTokens = intel.LexicalTokens
		record.NegativeTriggers = intel.NegativeTriggers
		record.Metrics = intel.Metrics
	}

	// 🛡️ ZERO-REFLECTION COERCION PRECOMPUTE: Natively pre-compute standard defaults off InputSchema so Squeezer bypasses reflection runtime natively
	if len(record.ZeroValues) == 0 {
		record.ZeroValues = ComputeZeroValues(record.InputSchema)
	}

	// Pop the cache for next time
	s.Cache.Set("tool:"+urn, &record, 2*time.Hour)
	return &record, nil
}

// ComputeZeroValues natively intercepts a JSONSchema parameter map extracting required structures
// substituting default or empty primitive proxies dynamically.
func ComputeZeroValues(schema map[string]any) map[string]any {
	zeroVals := make(map[string]any)
	if schema == nil {
		return zeroVals
	}

	reqRaw, ok := schema["required"]
	if !ok {
		return zeroVals
	}
	requiredArgs, ok := reqRaw.([]any)
	if !ok {
		return zeroVals
	}

	propsRaw, ok := schema["properties"]
	var props map[string]any
	if ok {
		props, _ = propsRaw.(map[string]any)
	}

	for _, reqIntf := range requiredArgs {
		key, ok := reqIntf.(string)
		if !ok {
			continue
		}

		var propType string
		if props != nil {
			if propDefRaw, hasProp := props[key]; hasProp {
				if propDef, ok := propDefRaw.(map[string]any); ok {
					if defRaw, hasDef := propDef["default"]; hasDef {
						zeroVals[key] = defRaw
						continue
					}
					if typeRaw, hasType := propDef["type"]; hasType {
						if typeStr, ok := typeRaw.(string); ok {
							propType = typeStr
						}
					}
				}
			}
		}

		switch propType {
		case "string":
			zeroVals[key] = ""
		case "integer", "number":
			zeroVals[key] = 0
		case "boolean":
			zeroVals[key] = false
		case "array":
			zeroVals[key] = []any{}
		case "object":
			zeroVals[key] = map[string]any{}
		default:
			zeroVals[key] = "" // fast, safe string natively
		}
	}
	return zeroVals
}

// SearchTools performs Keyword search on tool names and descriptions using Bleve index with optional category domain filtering.
// Results are re-ranked by blending BM25 relevance with usage frequency.
func (s *Store) SearchTools(ctx context.Context, query string, category string, serverConstraint string, scoreThreshold float64, alpha float64, domain SearchDomain) ([]*ToolRecord, error) {
	if query == "" {
		var results []*ToolRecord
		err := s.DB.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.DefaultIteratorOptions)
			defer it.Close()
			prefix := []byte("tool:")
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				valErr := item.Value(func(val []byte) error {
					var r ToolRecord
					if err := json.Unmarshal(val, &r); err == nil {
						if category != "" && !strings.EqualFold(r.Category, category) {
							return nil
						}
						if serverConstraint != "" && r.Server != serverConstraint {
							return nil
						}
						// 🛡️ DYNAMIC OVERLAY: Aggregate semantic parameters explicitly for bulk responses
						if intel, err := s.GetIntelligence(r.URN); err == nil && intel != nil {
							r.AnalysisStatus = intel.AnalysisStatus
							r.SyntheticIntents = intel.SyntheticIntents
							r.LexicalTokens = intel.LexicalTokens
							r.NegativeTriggers = intel.NegativeTriggers
							r.Metrics = intel.Metrics
						}
						results = append(results, &r)
					}
					return nil
				})
				if valErr != nil && valErr != badger.ErrDiscardedTxn {
					slog.Debug("failed to parse index tool", "error", valErr)
				}
			}
			return nil
		})
		return results, err
	}

	searchResult, err := s.Index.Search(query, category, serverConstraint, domain)
	if err != nil {
		return nil, err
	}

	// 🧠 HYBRID VECTOR OVERLAY: Fuse HNSW cosine similarity scores with Bleve BM25
	var vecResults []vector.ScoredResult
	if e := vector.GetEngine(); e != nil && e.VectorEnabled() && !strings.Contains(query, ":") {
		var vecErr error
		vecResults, vecErr = e.SearchWithScores(ctx, query, 20)
		if vecErr != nil {
			slog.Log(ctx, util.LevelTrace, "db: vector search unavailable, using pure BM25", "error", vecErr)
		}
	}

	if len(searchResult.Hits) == 0 && len(vecResults) == 0 {
		return s.SearchToolsFallback(query, category, serverConstraint, domain)
	}

	// Build unified hit list with blended scores
	type localHit struct {
		ID    string
		Score float64
	}
	var uniqueHits []localHit

	bm25Scores := make(map[string]float64)
	for _, hit := range searchResult.Hits {
		bm25Scores[hit.ID] = hit.Score
	}

	vectorScores := make(map[string]float64)
	for _, vr := range vecResults {
		// 🛡️ DOMAIN-AWARE SHARDING: Enforce strict visibility boundaries for vector results
		switch domain {
		case DomainUserLand:
			// Mask brainstorm and go-refactor unless explicitly targeted
			if serverConstraint == "" {
				if strings.HasPrefix(vr.Key, "brainstorm:") || strings.HasPrefix(vr.Key, "go-refactor:") {
					continue
				}
			}
		case DomainPipelineOrchestration:
			// Restrict to brainstorm, go-refactor, and magictools
			isPipeline := strings.HasPrefix(vr.Key, "brainstorm:") || strings.HasPrefix(vr.Key, "go-refactor:") || strings.HasPrefix(vr.Key, "magictools:")
			if !isPipeline {
				continue
			}
		case DomainSystem:
			// No sharding
		}

		if serverConstraint != "" {
			if !strings.HasPrefix(vr.Key, serverConstraint+":") && vr.Key != serverConstraint {
				continue
			}
		}
		vectorScores[vr.Key] = vr.Score
	}

	// Populate Top 5 Matrix for dashboard telemetry
	var bTop, hTop []string
	type scorePair struct {
		urn   string
		score float64
	}
	var bPairs []scorePair
	for u, s := range bm25Scores {
		bPairs = append(bPairs, scorePair{u, s})
	}
	for i := 1; i < len(bPairs); i++ {
		for j := i; j > 0 && bPairs[j].score > bPairs[j-1].score; j-- {
			bPairs[j], bPairs[j-1] = bPairs[j-1], bPairs[j]
		}
	}
	for i := 0; i < len(bPairs) && i < 5; i++ {
		bTop = append(bTop, bPairs[i].urn)
	}

	var hPairs []scorePair
	for u, s := range vectorScores {
		hPairs = append(hPairs, scorePair{u, s})
	}
	for i := 1; i < len(hPairs); i++ {
		for j := i; j > 0 && hPairs[j].score > hPairs[j-1].score; j-- {
			hPairs[j], hPairs[j-1] = hPairs[j-1], hPairs[j]
		}
	}
	for i := 0; i < len(hPairs) && i < 5; i++ {
		hTop = append(hTop, hPairs[i].urn)
	}

	telemetry.SearchMetrics.LastBleveTop5.Store(&bTop)
	telemetry.SearchMetrics.LastHnswTop5.Store(&hTop)

	allURNs := make(map[string]struct{})
	for urn := range bm25Scores {
		allURNs[urn] = struct{}{}
	}
	for urn := range vectorScores {
		allURNs[urn] = struct{}{}
	}

	bRank := make(map[string]int)
	for i, pair := range bPairs {
		bRank[pair.urn] = i + 1
	}

	hRank := make(map[string]int)
	for i, pair := range hPairs {
		hRank[pair.urn] = i + 1
	}

	fusedScores := make(map[string]float64)
	if len(hPairs) > 0 {
		// Hybrid Search RRF
		const k = 60.0
		for urn := range allURNs {
			var fused float64
			hasVec := false
			hasBM25 := false

			if r, ok := hRank[urn]; ok {
				fused += 1.0 / (k + float64(r))
				hasVec = true
			}
			if r, ok := bRank[urn]; ok {
				fused += 1.0 / (k + float64(r))
				hasBM25 = true
			}

			if hasVec && hasBM25 {
				if vectorScores[urn] > bm25Scores[urn] {
					telemetry.SearchMetrics.VectorWins.Add(1)
				} else {
					telemetry.SearchMetrics.LexicalWins.Add(1)
				}
			}
			// Scale up to approximate typical 0.0 - 1.0 range
			fusedScores[urn] = fused * 31.0
		}
	} else {
		// Pure BM25, no vectors
		for urn := range allURNs {
			if score, ok := bm25Scores[urn]; ok {
				fusedScores[urn] = score
			}
		}
	}

	// Determine the highest unified score to evaluate the threshold
	maxFusedScore := 0.0
	for _, score := range fusedScores {
		if score > maxFusedScore {
			maxFusedScore = score
		}
	}

	if scoreThreshold > 0 && maxFusedScore < scoreThreshold {
		fallback, err := s.SearchToolsFallback(query, category, serverConstraint, domain)
		if err == nil && len(fallback) > 0 {
			return fallback, nil
		}
		slog.Debug("High confidence threshold missed and fallback failed", "query", query)
	}

	var results []*ToolRecord
	seenNames := make(map[string]bool)

	for urn, fusedScore := range fusedScores {
		// 🛡️ STRICT CULLING: Drop tools that fall below the adjusted scoreThreshold individually.
		if scoreThreshold > 0 && fusedScore < (scoreThreshold*0.4) {
			continue
		}

		record, err := s.GetTool(urn)
		if err == nil {
			if serverConstraint == "" && seenNames[record.Name] {
				continue
			}
			if serverConstraint != "" && !strings.EqualFold(record.Server, serverConstraint) {
				continue
			}
			seenNames[record.Name] = true

			record.ConfidenceScore = fusedScore
			record.HighlightedDescription = record.Description

			// Try to recover fragment highlights from Bleve if available
			for _, hit := range searchResult.Hits {
				if hit.ID == urn {
					if frags, ok := hit.Fragments["description"]; ok && len(frags) > 0 {
						desc := frags[0]
						desc = strings.ReplaceAll(desc, "<mark>", "**")
						desc = strings.ReplaceAll(desc, "</mark>", "**")
						record.HighlightedDescription = desc
					}
					break
				}
			}

			results = append(results, record)
			uniqueHits = append(uniqueHits, localHit{ID: urn, Score: fusedScore})
		}
	}

	// Record confidence gap for collision dashboard
	if len(uniqueHits) >= 2 {
		// Sort just uniqueHits to find top 2 scores
		for i := 1; i < len(uniqueHits); i++ {
			for j := i; j > 0 && uniqueHits[j].Score > uniqueHits[j-1].Score; j-- {
				uniqueHits[j], uniqueHits[j-1] = uniqueHits[j-1], uniqueHits[j]
			}
		}
		s1 := uniqueHits[0]
		s2 := uniqueHits[1]
		gap := s1.Score - s2.Score
		telemetry.Collisions.Record(telemetry.CollisionEvent{
			Timestamp: time.Now().UnixNano(),
			Query:     query,
			S1URN:     s1.ID,
			S1Score:   s1.Score,
			S2URN:     s2.ID,
			S2Score:   s2.Score,
			Gap:       gap,
			Collision: gap < 0.05,
		})
	}

	// Usage-weighted re-ranking: blend Base position with usage frequency.
	if len(results) > 1 {
		var maxUsage int64
		for _, r := range results {
			if r.UsageCount > maxUsage {
				maxUsage = r.UsageCount
			}
		}

		if maxUsage > 0 {
			type scored struct {
				record *ToolRecord
				score  float64
			}
			scored_results := make([]scored, len(results))
			for i, r := range results {
				usageScore := 0.0
				if maxUsage > 0 {
					usageScore = math.Log10(float64(r.UsageCount)+1.0) / math.Log10(float64(maxUsage)+1.0)
				}
				baseScore := r.ConfidenceScore*0.7 + usageScore*0.3

				// 🛡️ Apply Incremental Reliability Scoring multiplier uniformly to Vector & BM25 hits
				rel := r.Metrics.ProxyReliability
				if rel == 0 {
					rel = 1.0
				}
				blended := baseScore * rel

				// 🛡️ Apply Semantic Negative Triggers Penalty uniformly
				if len(r.NegativeTriggers) > 0 {
					queryLower := strings.ToLower(query)
					for _, neg := range r.NegativeTriggers {
						if strings.Contains(queryLower, strings.ToLower(neg)) {
							blended = blended * 0.1
							break
						}
					}
				}

				r.ConfidenceScore = blended
				scored_results[i] = scored{record: r, score: blended}
			}

			// Sort by blended score descending
			for i := 1; i < len(scored_results); i++ {
				for j := i; j > 0 && scored_results[j].score > scored_results[j-1].score; j-- {
					scored_results[j], scored_results[j-1] = scored_results[j-1], scored_results[j]
				}
			}

			results = make([]*ToolRecord, 0, len(scored_results))
			serverCounts := make(map[string]int)
			for _, s := range scored_results {
				if serverConstraint != "" && !strings.EqualFold(s.record.Server, serverConstraint) {
					continue
				}
				if serverConstraint != "" || serverCounts[s.record.Server] < 3 {
					results = append(results, s.record)
					serverCounts[s.record.Server]++
				}
			}
		} else {
			// If no max usage, just sort by ConfidenceScore descending natively
			for i := 1; i < len(results); i++ {
				for j := i; j > 0 && results[j].ConfidenceScore > results[j-1].ConfidenceScore; j-- {
					results[j], results[j-1] = results[j-1], results[j]
				}
			}
		}
	} else if len(results) == 1 {
		r := results[0]
		rel := r.Metrics.ProxyReliability
		if rel == 0 {
			rel = 1.0
		}
		blended := r.ConfidenceScore * rel
		if len(r.NegativeTriggers) > 0 {
			queryLower := strings.ToLower(query)
			for _, neg := range r.NegativeTriggers {
				if strings.Contains(queryLower, strings.ToLower(neg)) {
					blended = blended * 0.1
					break
				}
			}
		}
		r.ConfidenceScore = blended
	}

	return results, nil
}

// SearchToolsFallback implements a pure linear substring scan across the BadgerDB key space.
// Used exclusively as a fallback when the Bleve search index returns zero hits or misses the threshold.
func (s *Store) SearchToolsFallback(query string, category string, serverConstraint string, domain SearchDomain) ([]*ToolRecord, error) {
	var results []*ToolRecord
	queryLower := strings.ToLower(query)
	categoryLower := strings.ToLower(category)
	serverLower := strings.ToLower(serverConstraint)

	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("tool:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			valErr := item.Value(func(val []byte) error {
				var r ToolRecord
				if err := json.Unmarshal(val, &r); err == nil {
					// Apply Category Filter
					if categoryLower != "" && strings.ToLower(r.Category) != categoryLower {
						return nil
					}
					// Apply Server Constraint Filter
					if serverLower != "" && strings.ToLower(r.Server) != serverLower {
						return nil
					}

					// 🛡️ DOMAIN-AWARE SHARDING: Enforce strict visibility boundaries
					switch domain {
					case DomainUserLand:
						if serverLower == "" && (r.Server == "brainstorm" || r.Server == "go-refactor") && !strings.Contains(queryLower, r.Server) {
							return nil
						}
					case DomainPipelineOrchestration:
						isPipeline := r.Server == "brainstorm" || r.Server == "go-refactor" || r.Server == "magictools"
						if !isPipeline {
							return nil
						}
					case DomainSystem:
						// No sharding
					}
					// Apply Word-Split Substring Match (any word matches)
					words := strings.Fields(queryLower)
					matched := false
					nameLower := strings.ToLower(r.Name)
					urnLower := strings.ToLower(r.URN)
					descLower := strings.ToLower(r.Description)
					for _, word := range words {
						if len(word) < 2 {
							continue
						}
						if strings.Contains(nameLower, word) ||
							strings.Contains(urnLower, word) ||
							strings.Contains(descLower, word) {
							matched = true
							break
						}
					}

					if matched {
						r.ConfidenceScore = 1.0 // Arbitrary fallback score
						// 🛡️ DYNAMIC OVERLAY: Aggregate semantic parameters explicitly for list arrays
						if intel, err := s.GetIntelligence(r.URN); err == nil && intel != nil {
							r.AnalysisStatus = intel.AnalysisStatus
							r.SyntheticIntents = intel.SyntheticIntents
							r.LexicalTokens = intel.LexicalTokens
							r.NegativeTriggers = intel.NegativeTriggers
							r.Metrics = intel.Metrics
						}
						results = append(results, &r)
					}
				}
				return nil
			})
			if valErr != nil && valErr != badger.ErrDiscardedTxn {
				slog.Debug("failed to parse fallback tool", "error", valErr)
			}
		}
		return nil
	})

	return results, err
}

// GetCategories returns all unique categories across all tools.
func (s *Store) GetCategories() ([]string, error) {
	// 1. Cache Check
	if cats, ok := s.Cache.GetCategories(); ok && len(cats) > 0 {
		return cats, nil
	}

	categories := make(map[string]struct{})
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("cat:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key() // cat:<category>:<urn>
			parts := strings.Split(string(key), ":")
			if len(parts) >= 2 {
				categories[parts[1]] = struct{}{}
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	results := make([]string, 0, len(categories))
	for k := range categories {
		results = append(results, k)
	}

	// Populate cache
	s.Cache.SetCategories(results)
	return results, nil
}

// SaveRawResource stores large tool outputs
func (s *Store) SaveRawResource(id string, data []byte) error {
	compressed, err := util.Compress(data)
	if err != nil {
		return err
	}

	return s.DB.Update(func(txn *badger.Txn) error {
		// 🛡️ FIX: Add TTL to prevent unbounded disk growth from cached proxy results.
		e := badger.NewEntry([]byte("raw:"+id), compressed).WithTTL(1 * time.Hour)
		return txn.SetEntry(e)
	})
}

// GetRawResource retrieves large tool outputs
func (s *Store) GetRawResource(id string) ([]byte, error) {
	var compressed []byte
	err := s.DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("raw:" + id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			compressed = append([]byte{}, val...)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return util.Decompress(compressed)
}

// UpdateToolUsage increments the usage counter for a tool
func (s *Store) UpdateToolUsage(urn string) {
	err := s.DB.Update(func(txn *badger.Txn) error {
		key := []byte("tool:" + urn)
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil // SILENT: key was likely purged during a sync/rename
			}
			return err
		}
		var r ToolRecord
		err = item.Value(func(val []byte) error {
			return json.Unmarshal(val, &r)
		})
		if err != nil {
			return err
		}

		r.UsageCount++
		r.LastUsedAt = time.Now().Unix()
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}

		return txn.Set(key, data)
	})

	if err != nil {
		slog.Warn("Failed to update tool usage stats", "urn", urn, "error", err)
	} else {
		// 🛡️ FIX: Invalidate cache AFTER successful commit to prevent stale data
		// when the transaction fails (e.g., conflict).
		s.Cache.Delete("tool:" + urn)
	}
}

// ReindexAllTools performs a full scan of Badger and updates the Bleve index.
// Used during lazy boot re-indexing and auto-heal recovery.
func (s *Store) ReindexAllTools() error {
	return s.DB.View(func(txn *badger.Txn) error {
		// 🛡️ PERF: Pre-load all intelligence records in a single scan to
		// eliminate N+1 GetIntelligence reads (one per tool). With ~200 tools,
		// this replaces ~200 Badger transactions with a single iterator pass.
		intelMap := make(map[string]*ToolIntelligence)
		intelIt := txn.NewIterator(badger.DefaultIteratorOptions)
		intelPrefix := []byte("intel:")
		for intelIt.Seek(intelPrefix); intelIt.ValidForPrefix(intelPrefix); intelIt.Next() {
			item := intelIt.Item()
			urn := strings.TrimPrefix(string(item.Key()), "intel:")
			_ = item.Value(func(val []byte) error {
				var ti ToolIntelligence
				if json.Unmarshal(val, &ti) == nil {
					intelMap[urn] = &ti
				}
				return nil
			})
		}
		intelIt.Close()

		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("tool:")
		var batch []BleveToolDocument

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var r ToolRecord
				if err := json.Unmarshal(val, &r); err == nil {
					// 🛡️ DYNAMIC OVERLAY: Aggregate semantic parameters for complete Bleve indexing
					if intel, ok := intelMap[r.URN]; ok && intel != nil {
						r.AnalysisStatus = intel.AnalysisStatus
						r.SyntheticIntents = intel.SyntheticIntents
						r.LexicalTokens = intel.LexicalTokens
						r.NegativeTriggers = intel.NegativeTriggers
						r.Metrics = intel.Metrics
					}
					batch = append(batch, ToBleveDoc(&r))

					// Flush batch when it hits 1000 to cap memory footprint
					if len(batch) >= 1000 {
						if err := s.Index.IndexBatch(batch); err != nil {
							slog.Warn("Failed to re-index tool batch in search index", "error", err)
						}
						batch = make([]BleveToolDocument, 0, 1000)
					}
				}
				return nil
			})
			if err != nil {
				slog.Error("Failed to read tool value during re-indexing", "key", string(item.Key()), "error", err)
			}
		}

		// Flush remaining batch elements
		if len(batch) > 0 {
			if err := s.Index.IndexBatch(batch); err != nil {
				slog.Warn("Failed to re-index final tool batch in search index", "error", err)
			}
		}

		return nil
	})
}

// PurgeServerTools removes all tool records for a specific server.
func (s *Store) PurgeServerTools(serverName string) error {
	var purgedTools int64
	var purgedIntel int64

	err := s.UpdateWithRetry(func(txn *badger.Txn) error {
		purgedTools = 0
		purgedIntel = 0

		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("tool:" + serverName + ":")
		var toDelete [][]byte

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.KeyCopy(nil)
			keyStr := string(key)

			err := item.Value(func(val []byte) error {
				var r ToolRecord
				if err := json.Unmarshal(val, &r); err != nil {
					// 🛡️ CORRUPT RECORD HANDLING: If JSON is invalid but the key
					// matches our server prefix, we MUST delete it anyway to prevent permanent orphans.
					slog.Warn("database: deleting corrupt tool record during purge", "key", keyStr)
					toDelete = append(toDelete, key)
					
					// Attempt to derive intelligence key safely
					if len(keyStr) > 5 && strings.HasPrefix(keyStr, "tool:") {
						urn := keyStr[5:]
						intelKey := []byte("intel:" + urn)
						toDelete = append(toDelete, intelKey)
					}
					return nil
				}
				
				if r.Server == serverName {
					toDelete = append(toDelete, key)
					// Also queue category index key for deletion
					catKey := []byte("cat:" + r.Category + ":" + r.URN)
					toDelete = append(toDelete, catKey)

					// Also queue intelligence record for deletion
					intelKey := []byte("intel:" + r.URN)
					toDelete = append(toDelete, intelKey)

					// Clear from Cache immediately
					s.Cache.Delete("tool:" + r.URN)
				}
				return nil
			})
			if err != nil {
				return err
			}
		}

		for _, key := range toDelete {
			if strings.HasPrefix(string(key), "tool:") {
				purgedTools++
			} else if strings.HasPrefix(string(key), "intel:") {
				purgedIntel++
			}
			if err := txn.Delete(key); err != nil {
				slog.Warn("failed to delete key during purge", "key", string(key), "error", err)
			}
			// If it's a tool record, remove from search index too
			keyStr := string(key)
			if len(keyStr) > 5 && keyStr[:5] == "tool:" {
				urn := keyStr[5:]
				if err := s.Index.DeleteRecord(urn); err != nil {
					slog.Warn("Failed to remove purged tool from search index", "urn", urn, "error", err)
				}
				if e := vector.GetEngine(); e != nil {
					e.DeleteDocument(urn)
				}
			}
		}

		if len(toDelete) > 0 {
			s.Cache.SetCategories(nil) // Invalidate category cache after purge
			slog.Info("purged tools for server", "server", serverName, "keys_deleted", len(toDelete))
		}
		return nil
	})

	if err == nil {
		if purgedTools > 0 {
			s.toolsCount.Add(-purgedTools)
		}
		if purgedIntel > 0 {
			s.intelCount.Add(-purgedIntel)
		}
	}
	return err
}

// PurgeStaleServerTools performs a delta-aware removal of tool records for a server.
// Only keys NOT present in validURNs are deleted. This is the safe counterpart to
// PurgeServerTools — designed for background execution after BatchSaveTools has already
// upserted the current tool set, preventing any zero-tools window.
func (s *Store) PurgeStaleServerTools(serverName string, validURNs []string) error {
	validMap := make(map[string]bool, len(validURNs))
	for _, urn := range validURNs {
		validMap[urn] = true
	}

	var purgedCount int64
	var purgedIntel int64

	err := s.UpdateWithRetry(func(txn *badger.Txn) error {
		purgedCount = 0 // Reset counter for retry safety
		purgedIntel = 0
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("tool:" + serverName + ":")
		type staleEntry struct {
			key    []byte
			urn    string
			catKey []byte
		}
		var toDelete []staleEntry

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			keyStr := string(item.Key())
			urn := keyStr[5:] // strip "tool:" prefix

			if validMap[urn] {
				continue // tool is current, keep it
			}

			entry := staleEntry{
				key: item.KeyCopy(nil),
				urn: urn,
			}

			// Extract category for cat: key cleanup
			_ = item.Value(func(val []byte) error {
				var r ToolRecord
				if err := json.Unmarshal(val, &r); err == nil {
					entry.catKey = []byte("cat:" + r.Category + ":" + r.URN)
				}
				return nil
			})

			toDelete = append(toDelete, entry)
		}

		for _, entry := range toDelete {
			purgedCount++
			if err := txn.Delete(entry.key); err != nil {
				slog.Warn("failed to delete stale tool key", "key", string(entry.key), "error", err)
			}
			if entry.catKey != nil {
				_ = txn.Delete(entry.catKey)
			}

			intelKey := []byte("intel:" + entry.urn)
			if _, gErr := txn.Get(intelKey); gErr == nil {
				purgedIntel++
				_ = txn.Delete(intelKey)
			}
			// Remove from search index
			if err := s.Index.DeleteRecord(entry.urn); err != nil {
				slog.Warn("failed to remove stale tool from search index", "urn", entry.urn, "error", err)
			}
			if e := vector.GetEngine(); e != nil {
				e.DeleteDocument(entry.urn)
			}
			// Clear cache
			s.Cache.Delete("tool:" + entry.urn)
		}

		if len(toDelete) > 0 {
			s.Cache.SetCategories(nil) // Invalidate category cache
			slog.Info("database: purged stale tools via delta-aware sweep", "server", serverName, "stale_keys", len(toDelete))
		}
		return nil
	})

	if err == nil {
		if purgedCount > 0 {
			s.toolsCount.Add(-purgedCount)
		}
		if purgedIntel > 0 {
			s.intelCount.Add(-purgedIntel)
		}
	}

	return err
}

// GetServerSyncHash retrieves the composite tool hash for a server from the last sync.
// Returns empty string if no hash is stored (first boot or schema change).
func (s *Store) GetServerSyncHash(server string) string {
	var hash string
	_ = s.DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("sync_hash:" + server))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			hash = string(val)
			return nil
		})
	})
	return hash
}

// SaveServerSyncHash persists the composite tool hash for a server.
func (s *Store) SaveServerSyncHash(server, hash string) {
	_ = s.UpdateWithRetry(func(txn *badger.Txn) error {
		return txn.Set([]byte("sync_hash:"+server), []byte(hash))
	})
}

// GetStaleServers returns a list of server names that have tools in the DB but are not in the activeNames list.
func (s *Store) GetStaleServers(activeNames []string) ([]string, error) {
	activeMap := make(map[string]bool, len(activeNames))
	for _, name := range activeNames {
		activeMap[name] = true
	}

	staleServers := make(map[string]bool)
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("tool:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := string(item.Key()) // format: tool:<server>:<name>
			parts := strings.Split(key, ":")
			if len(parts) >= 3 {
				serverName := parts[1]
				if !activeMap[serverName] {
					staleServers[serverName] = true
				}
			}
		}
		return nil
	})

	var stale []string
	for k := range staleServers {
		stale = append(stale, k)
	}
	return stale, err
}

// PurgeOrphanedServers finds and removes all tool records for servers not in the active list.
func (s *Store) PurgeOrphanedServers(activeNames []string) error {
	stale, err := s.GetStaleServers(activeNames)
	if err != nil {
		return err
	}
	for _, serverName := range stale {
		slog.Info("database: sweeping orphaned server records", "server", serverName)
		if err := s.PurgeServerTools(serverName); err != nil {
			slog.Warn("database: failed to purge orphaned server tools", "server", serverName, "error", err)
		}
		if err := s.PurgeServerIntelligence(serverName); err != nil {
			slog.Warn("database: failed to purge orphaned server intelligence", "server", serverName, "error", err)
		}
	}
	return nil
}

// PruneOrphanedIntelligence safely performs a delta-aware removal of orphaned semantic weights.
// It deletes intel:<serverName>:* keys that are NOT present in the validURNs slice.
func (s *Store) PruneOrphanedIntelligence(serverName string, validURNs []string) error {
	validMap := make(map[string]bool, len(validURNs))
	for _, urn := range validURNs {
		validMap[urn] = true
	}

	var purgedIntel int64

	err := s.UpdateWithRetry(func(txn *badger.Txn) error {
		purgedIntel = 0

		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("intel:" + serverName + ":")
		var toDelete [][]byte

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			keyStr := string(item.Key())

			// Format: intel:<server>:<tool>
			urn := keyStr[6:] // strip "intel:"

			if !validMap[urn] {
				toDelete = append(toDelete, item.KeyCopy(nil))
			}
		}

		for _, key := range toDelete {
			purgedIntel++
			if err := txn.Delete(key); err != nil {
				slog.Warn("failed to drop orphaned intelligence key", "key", string(key), "error", err)
			}
		}

		if len(toDelete) > 0 {
			slog.Info("database: gracefully pruned orphaned intelligence states", "server", serverName, "keys_deleted", len(toDelete))
		}
		return nil
	})

	if err == nil && purgedIntel > 0 {
		s.intelCount.Add(-purgedIntel)
	}
	return err
}

// PurgeServerIntelligence completely sweeps all LLM semantic states for a particular server namespace.
func (s *Store) PurgeServerIntelligence(serverName string) error {
	var purgedIntel int64

	err := s.UpdateWithRetry(func(txn *badger.Txn) error {
		purgedIntel = 0

		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("intel:" + serverName + ":")
		var toDelete [][]byte

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			toDelete = append(toDelete, item.KeyCopy(nil))
		}

		for _, key := range toDelete {
			purgedIntel++
			if err := txn.Delete(key); err != nil {
				slog.Warn("failed to drop server intelligence key", "key", string(key), "error", err)
			}
		}

		if len(toDelete) > 0 {
			slog.Info("database: wiped intelligence states for dropped server", "server", serverName, "keys_deleted", len(toDelete))
		}
		return nil
	})

	if err == nil && purgedIntel > 0 {
		s.intelCount.Add(-purgedIntel)
	}
	return err
}

// ReconcileMetrics forces full cross-namespace parity between tool: and intel: keys.
// It deletes orphaned intel: keys that have no corresponding tool: key, then recalibrates
// the atomic counters from actual database state. This is the authoritative consistency gate.
func (s *Store) ReconcileMetrics() (orphansDeleted int64, err error) {
	// Phase 1: Collect all valid tool URNs in a read-only scan.
	validTools := make(map[string]bool)
	err = s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("tool:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			keyStr := string(it.Item().Key())
			urn := keyStr[5:] // strip "tool:" → "server:name"
			validTools[urn] = true
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("reconcile: failed to scan tool namespace: %w", err)
	}

	// Phase 2: Find orphaned intel: keys that have no matching tool: key.
	var orphanedIntelKeys [][]byte
	err = s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("intel:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			keyStr := string(it.Item().Key())
			urn := keyStr[6:] // strip "intel:" → "server:name"
			if !validTools[urn] {
				orphanedIntelKeys = append(orphanedIntelKeys, it.Item().KeyCopy(nil))
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("reconcile: failed to scan intel namespace: %w", err)
	}

	// Phase 3: Delete orphaned intel keys.
	if len(orphanedIntelKeys) > 0 {
		err = s.UpdateWithRetry(func(txn *badger.Txn) error {
			for _, key := range orphanedIntelKeys {
				if delErr := txn.Delete(key); delErr != nil {
					slog.Warn("reconcile: failed to delete orphaned intel key", "key", string(key), "error", delErr)
				}
			}
			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("reconcile: failed to purge orphaned intel keys: %w", err)
		}
		orphansDeleted = int64(len(orphanedIntelKeys))
	}

	// Phase 4: Recalibrate atomic counters from actual key counts.
	toolCount, _ := s.countKeys("tool:")
	intelCount, _ := s.countKeys("intel:")
	s.toolsCount.Store(int64(toolCount))
	s.intelCount.Store(int64(intelCount))

	slog.Info("database: reconciliation complete",
		"orphans_deleted", orphansDeleted,
		"tool_count", toolCount,
		"intel_count", intelCount,
	)
	return orphansDeleted, nil
}

// HasServerTools checks if any tools exist for the given server using a prefix search.
func (s *Store) HasServerTools(serverName string) bool {
	found := false
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("tool:" + serverName + ":")
		it.Seek(prefix)
		if it.ValidForPrefix(prefix) {
			found = true
		}
		return nil
	})
	if err != nil {
		slog.Error("Failed to check for server tools existence", "server", serverName, "error", err)
	}
	return found
}

// GetServerToolCount returns the number of tools indexed for a specific server.
func (s *Store) GetServerToolCount(serverName string) int {
	count := 0
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("tool:" + serverName + ":")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			count++
		}
		return nil
	})
	if err != nil {
		slog.Error("Failed to count server tools", "server", serverName, "error", err)
	}
	return count
}

// GetServerToolsNatively directly scans the LSM tree for a server's tools, bypassing Bleve entirely.
func (s *Store) GetServerToolsNatively(serverName string, limit int) ([]*ToolRecord, error) {
	var tools []*ToolRecord
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("tool:" + serverName + ":")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			err := it.Item().Value(func(val []byte) error {
				var r ToolRecord
				if err := json.Unmarshal(val, &r); err != nil {
					return err
				}
				tools = append(tools, &r)
				return nil
			})
			if err != nil {
				return err
			}
			if limit > 0 && len(tools) >= limit {
				break
			}
		}
		return nil
	})
	return tools, err
}

// GetAllToolURNs returns a map of all tool URNs currently stored in BadgerDB.
// Uses key-only iteration (PrefetchValues=false) for minimal overhead.
func (s *Store) GetAllToolURNs() map[string]bool {
	urns := make(map[string]bool)
	_ = s.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte("tool:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			urn := key[5:] // strip "tool:" prefix
			urns[urn] = true
		}
		return nil
	})
	return urns
}

// SaveLog persists a log entry with a TTL for self-cleaning.
func (s *Store) SaveLog(entry []byte, ttl time.Duration) error {
	// Use UnixNano for chronological sorting (ascending by default in Badger)
	timestamp := time.Now().UnixNano()
	key := fmt.Appendf(nil, "log:%020d", timestamp)
	return s.DB.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry(key, entry).WithTTL(ttl)
		return txn.SetEntry(e)
	})
}

// GetLogs retrieves the most recent log entries up to maxLines.
func (s *Store) GetLogs(maxLines int) ([]string, error) {
	var logs []string
	err := s.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Reverse = true // Latest logs first
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("log:")
		// Seek to the very end of the log prefix range
		it.Seek([]byte("log:\xff"))

		for ; it.ValidForPrefix(prefix) && len(logs) < maxLines; it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				logs = append(logs, string(val))
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	// Reverse the slice back to chronological order (Ascending)
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
	return logs, err
}

// DiagnosticStats is undocumented but satisfies standard structural requirements.
type DiagnosticStats struct {
	TotalKeys       int    `json:"total_keys"`
	LSMSize         int64  `json:"lsm_size_bytes"`
	VLogSize        int64  `json:"vlog_size_bytes"`
	TTLKeysTotal    int    `json:"ttl_keys_total"`
	TTLKeysUnder1H  int    `json:"ttl_keys_under_1h"`
	TTLKeysUnder24H int    `json:"ttl_keys_under_24h"`
	SyncState       string `json:"sync_state"`
}

// GetExtendedDiagnostics streams the Badger DB evaluating TTLs and Bleve index parity
func (s *Store) GetExtendedDiagnostics() (*DiagnosticStats, error) {
	stats := &DiagnosticStats{
		SyncState: "SYNCED",
	}

	// 1. Get raw disk size sizes
	lsm, vlog := s.DB.Size()
	stats.LSMSize = lsm
	stats.VLogSize = vlog

	// 2. Iterate keys for TTLs and sync parity
	now := uint64(time.Now().Unix())
	oneHour := now + 3600
	oneDay := now + 86400

	err := s.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // High speed iteration
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := string(item.Key())

			if strings.HasPrefix(key, "tool:") {
				stats.TotalKeys++
			}

			// TTL analysis
			if expires := item.ExpiresAt(); expires > 0 {
				stats.TTLKeysTotal++
				if expires <= oneHour {
					stats.TTLKeysUnder1H++
				}
				if expires <= oneDay {
					stats.TTLKeysUnder24H++
				}
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// 3. Bleve DocCount Parity
	if idx, ok := s.Index.index.Load().(bleve.Index); ok && idx != nil {
		dCount, err := idx.DocCount()
		if err == nil {
			if int(dCount) != stats.TotalKeys {
				stats.SyncState = "OUT_OF_SYNC"
			}
		} else {
			stats.SyncState = "INDEX_ERROR"
		}
	} else {
		stats.SyncState = "INDEX_UNAVAILABLE"
	}

	// Update telemetry global sync state
	if stats.SyncState == "OUT_OF_SYNC" {
		telemetry.SyncOutOfSync.Store(true)
	}

	return stats, nil
}

// SaveTrigger persists a keyword-to-server mapping for predictive discovery.
func (s *Store) SaveTrigger(keyword, server string) error {
	return s.DB.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("trigger:"+keyword), []byte(server))
	})
}

// PopulateDefaultTriggers seeds the trigger DB with keyword→server mappings.
// This replaces the hardcoded isPreferred() function with data-driven steering.
// Safe to call multiple times — triggers are upserted (last write wins).
func (s *Store) PopulateDefaultTriggers() {
	defaults := map[string]string{
		// CI/CD & Deployment
		"deploy":   "glab",
		"pipeline": "glab",
		"ci":       "glab",
		"cd":       "glab",
		"release":  "glab",
		// Debugging & Maintenance
		"fix":   "go-refactor",
		"bug":   "go-refactor",
		"debug": "go-refactor",
		"test":  "go-refactor",
		// Refactoring & Clean Code
		"refactor":  "go-refactor",
		"modernize": "go-refactor",
		"clean":     "go-refactor",
		// External Knowledge & Research
		"search":   "ddg-search",
		"web":      "ddg-search",
		"lookup":   "ddg-search",
		"research": "ddg-search",
		// Architecture & Planning
		"design":       "brainstorm",
		"architecture": "brainstorm",
		"plan":         "brainstorm",
		"critique":     "brainstorm",
		// Sequential Thinking
		"think":      "seq-thinking",
		"thinking":   "seq-thinking",
		"reason":     "seq-thinking",
		"reasoning":  "seq-thinking",
		"analyze":    "seq-thinking",
		"sequential": "seq-thinking",
		// Skills & Workflow
		"skill":     "magicskills",
		"workflow":  "magicskills",
		"bootstrap": "magicskills",
		// Context Preservation & Memory
		"memory":   "recall",
		"recall":   "recall",
		"remember": "recall",
		"context":  "recall",
		// File Operations
		"file":      "filesystem",
		"directory": "filesystem",
		"folder":    "filesystem",
		"path":      "filesystem",
		// Version Control
		"git":    "git",
		"commit": "git",
		"branch": "git",
		"merge":  "git",
		"diff":   "git",
	}

	for keyword, server := range defaults {
		if err := s.SaveTrigger(keyword, server); err != nil {
			slog.Warn("Failed to save default trigger", "keyword", keyword, "server", server, "error", err)
		}
	}
	slog.Info("database: populated default triggers", "count", len(defaults))
}

// GetTriggers returns all keyword-to-server mappings.
func (s *Store) GetTriggers() (map[string]string, error) {
	triggers := make(map[string]string)
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("trigger:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := string(item.Key()[8:])
			err := item.Value(func(val []byte) error {
				triggers[key] = string(val)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	return triggers, err
}

// AnalyzeIntent performs a fast regex-based scan of a message against the trigger map.
func (s *Store) AnalyzeIntent(ctx context.Context, msg string) []string {
	triggers, err := s.GetTriggers()
	if err != nil || len(triggers) == 0 {
		return nil
	}

	matches := make(map[string]struct{})
	msgLower := strings.ToLower(msg)

	for kw, server := range triggers {
		// Use regex for word-boundary matching to prevent false positives (e.g., 'edit' in 'edition')
		pattern := fmt.Sprintf(`(?i)\b%s\b`, regexp.QuoteMeta(kw))
		if re, err := regexp.Compile(pattern); err == nil {
			if re.MatchString(msgLower) {
				matches[server] = struct{}{}
			}
		} else if strings.Contains(msgLower, strings.ToLower(kw)) {
			// Fallback to simple contains
			matches[server] = struct{}{}
		}
	}

	var result []string
	for k := range matches {
		result = append(result, k)
	}
	return result
}

// GetTopToolsForServer retrieves full mcp.Tool schemas for the top utilized tools of a server.
func (s *Store) GetTopToolsForServer(server string, max int) ([]mcp.Tool, error) {
	var records []*ToolRecord
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("tool:" + server + ":")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var r ToolRecord
				if err := json.Unmarshal(val, &r); err == nil {
					records = append(records, &r)
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort by usage count descending
	sortRecords(records)

	if len(records) > max {
		records = records[:max]
	}

	var tools []mcp.Tool
	for _, r := range records {
		schema, _ := s.GetSchema(r.SchemaHash)
		tools = append(tools, mcp.Tool{
			Name:        r.Name,
			Description: r.Description,
			InputSchema: schema,
		})
	}
	return tools, nil
}

func sortRecords(records []*ToolRecord) {
	// Simple bubble sort for usage count descending since result sets are small (per-server tools)
	for i := range records {
		for j := i + 1; j < len(records); j++ {
			if records[i].UsageCount < records[j].UsageCount {
				records[i], records[j] = records[j], records[i]
			}
		}
	}
}

// WipeAll clears EVERYTHING from the persistent store (Badger) and search index (Bleve).
// This is used for hard-resets or when the database becomes corrupted.
func (s *Store) WipeAll() error {
	slog.Warn("database: initiating COMPLETE data wipe")

	// 1. Drop all data from Badger
	if err := s.DB.DropAll(); err != nil {
		return fmt.Errorf("failed to drop badger data: %w", err)
	}

	// 2. Re-initialize Search Index
	if s.Index != nil {
		if err := s.Index.Close(); err != nil {
			slog.Error("database: wipe failed to close search index", "error", err)
		}
		indexPath := filepath.Join(s.Path, "index.bleve")
		if err := os.RemoveAll(indexPath); err != nil {
			slog.Warn("database: wipe failed to remove search index directory", "path", indexPath, "error", err)
		}

		newIndex, err := NewSearchIndex(s.Path)
		if err != nil {
			return fmt.Errorf("failed to re-initialize search index: %w", err)
		}
		s.Index = newIndex
	}

	// 3. Clear in-memory cache to prevent stale GetTool hits
	s.Cache.Clear()

	return nil
}

// ComputeScoreBoard builds tool score cards from live GlobalToolTracker data,
// overlays intel baselines, and merges pre-computed trending deltas.
// Called on every health tick for real-time scores.
func (s *Store) ComputeScoreBoard(trending map[string]map[string]float64) map[string]any {
	scores := make(map[string]any)

	// 1. Primary source: live call/fault/latency from GlobalToolTracker
	liveTools := telemetry.GlobalToolTracker.GetAll()
	for urn, m := range liveTools {
		if m.Calls == 0 {
			continue
		}
		reliability := float64(m.Calls-m.Faults) / float64(m.Calls)
		avgMs := m.TotalMs / m.Calls

		card := map[string]any{
			"URN":         urn,
			"Calls":       m.Calls,
			"Faults":      m.Faults,
			"Reliability": reliability,
			"AvgMs":       avgMs,
			"Baseline":    1.0,
			"Deviation":   reliability - 1.0,
			"Delta30m":    0.0,
			"Delta4h":     0.0,
			"DeltaAll":    0.0,
		}

		// Overlay trending if available
		if t, ok := trending[urn]; ok {
			card["Delta30m"] = t["Delta30m"]
			card["Delta4h"] = t["Delta4h"]
			card["DeltaAll"] = t["DeltaAll"]
		}

		scores[urn] = card
	}

	// 2. Overlay intel baselines where available
	if s != nil && s.DB != nil {
		intelPrefix := []byte("intel:")
		_ = s.DB.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			opts.PrefetchValues = true
			opts.PrefetchSize = 50
			opts.Prefix = intelPrefix
			it := txn.NewIterator(opts)
			defer it.Close()

			for it.Seek(intelPrefix); it.ValidForPrefix(intelPrefix); it.Next() {
				item := it.Item()
				urn := strings.TrimPrefix(string(item.Key()), "intel:")

				cardRaw, exists := scores[urn]
				if !exists {
					continue // only overlay tools we already have live data for
				}
				card, ok := cardRaw.(map[string]any)
				if !ok {
					continue
				}

				_ = item.Value(func(val []byte) error {
					var intel map[string]any
					if err := json.Unmarshal(val, &intel); err == nil {
						if metrics, ok := intel["metrics"].(map[string]any); ok {
							if rel, ok := metrics["proxy_reliability"].(float64); ok && rel > 0 {
								card["Baseline"] = rel
								if r, ok := card["Reliability"].(float64); ok {
									card["Deviation"] = r - rel
								}
							}
						}
					}
					return nil
				})
			}
			return nil
		})
	}

	// 3. Cap to top 20 by call count for gauge size safety
	if len(scores) > 20 {
		type urnCalls struct {
			urn   string
			calls int64
		}
		var sorted []urnCalls
		for urn, raw := range scores {
			if card, ok := raw.(map[string]any); ok {
				if c, ok := card["Calls"].(int64); ok {
					sorted = append(sorted, urnCalls{urn, c})
				}
			}
		}
		// Simple sort descending by calls
		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[i].calls < sorted[j].calls {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		keep := make(map[string]bool)
		for i := 0; i < 20 && i < len(sorted); i++ {
			keep[sorted[i].urn] = true
		}
		for urn := range scores {
			if !keep[urn] {
				delete(scores, urn)
			}
		}
	}

	return scores
}

// ComputeTrending scans BadgerDB telemetry:tool:* history to compute trending
// deltas for 30m, 4h, and all-time windows. Called on flush ticks only (every 1 min).
func (s *Store) ComputeTrending() map[string]map[string]float64 {
	trending := make(map[string]map[string]float64)

	if s == nil || s.DB == nil {
		return trending
	}

	now := time.Now().Unix()
	win30m := now - (30 * 60)
	win4h := now - (4 * 3600)

	// Per-URN: track earliest/latest calls+faults per window
	type snapshot struct {
		ts     int64
		calls  int64
		faults int64
	}
	type windowData struct {
		earliest30m *snapshot
		latest30m   *snapshot
		earliest4h  *snapshot
		latest4h    *snapshot
		earliestAll *snapshot
		latestAll   *snapshot
	}
	byURN := make(map[string]*windowData)

	historyPrefix := []byte("telemetry:tool:")
	_ = s.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		opts.Prefix = historyPrefix
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(historyPrefix); it.ValidForPrefix(historyPrefix); it.Next() {
			item := it.Item()
			key := string(item.Key())

			rest := strings.TrimPrefix(key, "telemetry:tool:")
			lastColon := strings.LastIndex(rest, ":")
			if lastColon <= 0 {
				continue
			}
			urn := rest[:lastColon]
			var ts int64
			fmt.Sscanf(rest[lastColon+1:], "%d", &ts)

			_ = item.Value(func(val []byte) error {
				var entry map[string]any
				if err := json.Unmarshal(val, &entry); err != nil {
					return nil
				}
				var calls, faults int64
				if v, ok := entry["calls"].(float64); ok {
					calls = int64(v)
				}
				if v, ok := entry["faults"].(float64); ok {
					faults = int64(v)
				}

				w, exists := byURN[urn]
				if !exists {
					w = &windowData{}
					byURN[urn] = w
				}
				s := &snapshot{ts: ts, calls: calls, faults: faults}

				// All-time
				if w.earliestAll == nil || ts < w.earliestAll.ts {
					w.earliestAll = s
				}
				if w.latestAll == nil || ts > w.latestAll.ts {
					w.latestAll = s
				}

				// 30m window
				if ts >= win30m {
					if w.earliest30m == nil || ts < w.earliest30m.ts {
						w.earliest30m = s
					}
					if w.latest30m == nil || ts > w.latest30m.ts {
						w.latest30m = s
					}
				}

				// 4h window
				if ts >= win4h {
					if w.earliest4h == nil || ts < w.earliest4h.ts {
						w.earliest4h = s
					}
					if w.latest4h == nil || ts > w.latest4h.ts {
						w.latest4h = s
					}
				}

				return nil
			})
		}
		return nil
	})

	// Compute deltas from window boundaries
	reliabilityOf := func(s *snapshot) float64 {
		if s == nil || s.calls == 0 {
			return 1.0
		}
		return float64(s.calls-s.faults) / float64(s.calls)
	}

	for urn, w := range byURN {
		t := map[string]float64{
			"Delta30m": 0.0,
			"Delta4h":  0.0,
			"DeltaAll": 0.0,
		}
		if w.earliest30m != nil && w.latest30m != nil {
			t["Delta30m"] = reliabilityOf(w.latest30m) - reliabilityOf(w.earliest30m)
		}
		if w.earliest4h != nil && w.latest4h != nil {
			t["Delta4h"] = reliabilityOf(w.latest4h) - reliabilityOf(w.earliest4h)
		}
		if w.earliestAll != nil && w.latestAll != nil {
			t["DeltaAll"] = reliabilityOf(w.latestAll) - reliabilityOf(w.earliestAll)
		}
		trending[urn] = t
	}

	return trending
}

// WipeDatabase permanently drops all tools, intelligence, and resets searches
func (s *Store) WipeDatabase() error {
	// 1. Drop Badger KV space
	if err := s.DB.DropAll(); err != nil {
		return fmt.Errorf("failed to drop KV store: %w", err)
	}

	// 2. Drop Bleve search
	if s.Index != nil {
		s.Index.Close()
		idxPath := filepath.Join(s.Path, "search.idx")
		os.RemoveAll(idxPath)
		newIndex, err := NewSearchIndex(s.Path)
		if err != nil {
			return fmt.Errorf("failed to re-initialize search index: %w", err)
		}
		s.Index = newIndex
	}

	// 3. Clear Caches
	s.Cache.Clear()

	slog.Info("database: COMPLETE data wipe successful")
	return nil
}

// ── Databases TUI Trending ─────────────────────────────────────────────────

// ComputeDatabaseTrending retrieves exact historic database snapshots for 5m, 15m, 1h windows
// directly from the memory-mapped badger flush buckets to allow the TUI to render velocity rates natively
func (s *Store) ComputeDatabaseTrending() map[string]any {
	trending := make(map[string]any)
	if s == nil || s.DB == nil {
		return trending
	}

	now := time.Now().Unix()
	windows := []struct {
		Name   string
		Target int64
	}{
		{"5m", now - (5 * 60)},
		{"15m", now - (15 * 60)},
		{"1h", now - (60 * 60)},
	}

	_ = s.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		opts.Reverse = true // Scan backwards from the target

		for _, w := range windows {
			it := txn.NewIterator(opts)
			seekKey := fmt.Sprintf("telemetry:bucket:%d", w.Target)
			it.Seek([]byte(seekKey))

			if it.ValidForPrefix([]byte("telemetry:bucket:")) {
				item := it.Item()
				_ = item.Value(func(val []byte) error {
					var snapshot map[string]any
					if json.Unmarshal(val, &snapshot) == nil {
						if dbs, ok := snapshot["databases"]; ok {
							trending[w.Name] = dbs
						}
					}
					return nil
				})
			}
			it.Close()
		}
		return nil
	})

	return trending
}
