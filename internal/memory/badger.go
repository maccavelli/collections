package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
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
func (s *MemoryStore) Save(ctx context.Context, key, content string, tags []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec := &Record{
		Content:   content,
		Tags:      tags,
		UpdatedAt: time.Now(),
	}

	var oldRec *Record
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err == nil {
			_ = item.Value(func(val []byte) error {
				if old, err := migrateRecord(val); err == nil {
					oldRec = old
					rec.CreatedAt = old.CreatedAt
				}
				return nil
			})
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("lookup failure: %w", err)
	}

	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = rec.UpdatedAt
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal failure: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		// Remove old index entries if they exist
		if oldRec != nil {
			oldIdxKey := fmt.Sprintf("_idx:t:%x:%s", oldRec.UpdatedAt.UnixNano(), key)
			_ = txn.Delete([]byte(oldIdxKey))
		}

		// Save record content
		if err := txn.Set([]byte(key), data); err != nil {
			return err
		}

		// Save secondary index for time-based lookups (descending sort friendly)
		// We use a hex-encoded timestamp to ensure binary sortability
		idxKey := fmt.Sprintf("_idx:t:%x:%s", rec.UpdatedAt.UnixNano(), key)
		return txn.Set([]byte(idxKey), []byte(key))
	})

	if err == nil {
		slog.Debug("Memory saved and indexed", "key", key, "tag_count", len(tags))
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

// Search matches keys, content, and tags with context awareness and limits.
func (s *MemoryStore) Search(ctx context.Context, query string, tagFilter string, limit int) (map[string]*Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make(map[string]*Record)
	query = strings.ToLower(query)
	tagFilter = strings.ToLower(tagFilter)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			// Hardening: Respect context cancellation during long scans
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			item := it.Item()
			k := string(item.Key())

			// Skip index entries
			if strings.HasPrefix(k, "_idx:") {
				continue
			}

			if limit > 0 && len(results) >= limit {
				break
			}
			
			err := item.Value(func(v []byte) error {
				rec, err := migrateRecord(v)
				if err != nil {
					return nil
				}

				if !s.matchesFilter(k, rec, query, tagFilter) {
					return nil
				}

				results[k] = rec
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	return results, err
}

// matchesFilter checks if a record satisfies keyword and tag requirements.
func (s *MemoryStore) matchesFilter(key string, rec *Record, query, tagFilter string) bool {
	// Filter by tag if provided
	if tagFilter != "" {
		found := false
		for _, t := range rec.Tags {
			if strings.ToLower(t) == tagFilter {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by keyword if provided
	if query == "" {
		return true
	}

	if strings.Contains(strings.ToLower(key), query) || 
	   strings.Contains(strings.ToLower(rec.Content), query) {
		return true
	}

	for _, t := range rec.Tags {
		if strings.Contains(strings.ToLower(t), query) {
			return true
		}
	}

	return false
}

// GetRecent retrieves the last N memories sorted by UpdatedAt descending.
// Optimized: Uses the secondary timestamp index for O(K) retrieval.
func (s *MemoryStore) GetRecent(ctx context.Context, count int) (map[string]*Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make(map[string]*Record)
	if count <= 0 {
		return results, nil
	}

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Reverse = true // Latest first
		
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("_idx:t:")
		// In reverse mode, Seek to prefix plus a high byte to start at the end of the index range
		seekKey := append([]byte(nil), prefix...)
		seekKey = append(seekKey, 0xff, 0xff, 0xff)

		for it.Seek(seekKey); it.ValidForPrefix(prefix) && len(results) < count; it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				// The value of the index is the original key
				originalKey := val
				
				item, err := txn.Get(originalKey)
				if err != nil {
					return nil // Skip if missing
				}

				return item.Value(func(v []byte) error {
					if rec, err := migrateRecord(v); err == nil {
						results[string(originalKey)] = rec
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
func (s *MemoryStore) ListKeys(ctx context.Context) (map[string]*Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make(map[string]*Record)
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
					results[k] = rec
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
			idxKey := fmt.Sprintf("_idx:t:%x:%s", rec.UpdatedAt.UnixNano(), key)
			_ = txn.Delete([]byte(idxKey))
		}
		return txn.Delete([]byte(key))
	})
}

// Close safely shuts down the database and maintenance routines.
func (s *MemoryStore) Close() error {
	close(s.stopGC)
	return s.db.Close()
}
