package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
)

type EfficacyStats struct {
	Successes     int       `json:"successes"`
	Failures      int       `json:"failures"`
	LastSuccessAt time.Time `json:"last_success_at,omitempty"`
	LastFailureAt time.Time `json:"last_failure_at,omitempty"`
	LastUsedAt    time.Time `json:"last_used_at"`
}

type Store struct {
	db     *badger.DB
	ctx    context.Context
	cancel context.CancelFunc
	hits   uint64
	misses uint64
}

func NewStore(dbPath string) (*Store, error) {
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	opts := badger.DefaultOptions(dbPath).
		WithLogger(nil).
		WithSyncWrites(false).
		WithCompression(options.None).
		WithValueThreshold(1 << 20).                      // 1MB — keeps values in LSM tree
		WithValueLogFileSize(4 << 20).                    // 4MB — aggressive rotation
		WithValueLogMaxEntries(100).                      // Aggressive rotation
		WithNumVersionsToKeep(1).                         // Keep only latest version
		WithIndexCacheSize(0).                            // 0MB (Disabled) - Rely on OS Page Cache
		WithBlockCacheSize(0).                            // 0MB (Disabled) - Rely on OS Page Cache
		WithMemTableSize(8 << 20).                        // 8MB
		WithNumMemtables(1).                              // 1
		WithNumLevelZeroTables(2).                        // 2
		WithNumLevelZeroTablesStall(4).                   // 4
		WithCompactL0OnClose(true).                       // 🛡️ F4: Compact L0 on shutdown
		WithDetectConflicts(false).                       // 🛡️ F7: Single-writer workload
		WithChecksumVerificationMode(options.OnTableRead) // 🛡️ F5: Verify SST checksums on open

	// 🛡️ F6: Lock retry with exponential backoff (matching recall's pattern)
	var db *badger.DB
	var err error
	maxRetries := 5
	backoff := 500 * time.Millisecond
	for i := 0; i < maxRetries; i++ {
		db, err = badger.Open(opts)
		if err == nil {
			break
		}
		if strings.Contains(strings.ToLower(err.Error()), "cannot acquire directory lock") {
			slog.Warn("Badger directory lock held; retrying...", "attempt", i+1, "max_retries", maxRetries, "backoff", backoff)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db after %d retries: %w", maxRetries, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Store{db: db, ctx: ctx, cancel: cancel}
	go s.runGC(ctx)

	slog.Info("BadgerDB initialized", "path", dbPath)
	return s, nil
}

// runGC executes BadgerDB value log garbage collection periodically.
// 🛡️ F3: Uses select on stopGC channel for graceful shutdown (no goroutine leak).
func (s *Store) runGC(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for {
				if err := s.db.RunValueLogGC(0.5); err != nil {
					break // ErrNoRewrite or other error stops GC
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.cancel()
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) RecordEfficacy(workspaceRoot, skillName string, success bool) error {
	key := []byte(fmt.Sprintf("%s:eff:%s", workspaceRoot, skillName))
	return s.db.Update(func(txn *badger.Txn) error {
		var stats EfficacyStats
		item, err := txn.Get(key)
		if err == nil {
			err = item.Value(func(val []byte) error {
				return json.Unmarshal(val, &stats)
			})
			if err != nil {
				return err
			}
		} else if err != badger.ErrKeyNotFound {
			return err
		}

		now := time.Now()
		stats.LastUsedAt = now
		if success {
			stats.Successes++
			stats.LastSuccessAt = now
		} else {
			stats.Failures++
			stats.LastFailureAt = now
		}

		data, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		return txn.Set(key, data)
	})
}

func (s *Store) GetEfficacy(workspaceRoot, skillName string) (EfficacyStats, error) {
	var stats EfficacyStats
	key := []byte(fmt.Sprintf("%s:eff:%s", workspaceRoot, skillName))
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &stats)
		})
	})
	if err == badger.ErrKeyNotFound {
		atomic.AddUint64(&s.misses, 1)
		return stats, nil
	}
	if err == nil {
		atomic.AddUint64(&s.hits, 1)
	}
	return stats, err
}

func (s *Store) GetMetrics() (uint64, uint64) {
	return atomic.LoadUint64(&s.hits), atomic.LoadUint64(&s.misses)
}

// CountEntries performs a high-speed, zero-RAM count of all physical keys in BadgerDB.
func (s *Store) CountEntries() uint64 {
	var count uint64
	_ = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Crucial: avoid loading values into memory
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			count++
		}
		return nil
	})
	return count
}
