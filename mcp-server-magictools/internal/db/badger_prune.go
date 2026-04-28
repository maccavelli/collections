package db

import (
	"encoding/json"
	"log/slog"

	"github.com/dgraph-io/badger/v4"
)

// PruneOrphans scans the badger tool namespace and purges any tools where the originating server is no longer registered.
func (s *Store) PruneOrphans(activeServers []string) (int, error) {
	if s == nil || s.DB == nil {
		return 0, nil
	}

	valid := make(map[string]bool)
	for _, srv := range activeServers {
		valid[srv] = true
	}
	valid["magictools"] = true

	var orphanedKeys [][]byte
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("tool:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.KeyCopy(nil)
			_ = item.Value(func(val []byte) error {
				var r ToolRecord
				if err := json.Unmarshal(val, &r); err == nil {
					if !valid[r.Server] {
						orphanedKeys = append(orphanedKeys, key)
					}
				}
				return nil
			})
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	if len(orphanedKeys) == 0 {
		return 0, nil
	}

	err = s.UpdateWithRetry(func(txn *badger.Txn) error {
		for _, key := range orphanedKeys {
			urn := string(key)[5:] // Strip "tool:"

			// Decrement global tools count safely natively
			s.toolsCount.Add(-1)

			// Delete tool record
			if err := txn.Delete(key); err != nil {
				return err
			}

			// Delete matching intelligence if exists
			intelKey := []byte("intel:" + urn)
			if _, gErr := txn.Get(intelKey); gErr == nil {
				s.intelCount.Add(-1)
				_ = txn.Delete(intelKey)
			}

			// Reconcile Bleve index explicitly
			if s.Index != nil {
				_ = s.Index.DeleteRecord(urn)
			}
			s.Cache.Delete(string(key))
		}
		return nil
	})

	if err == nil {
		slog.Info("database: pruned orphaned tools structurally aligning metrics", "count", len(orphanedKeys))
		s.Cache.SetCategories(nil)
	} else {
		slog.Error("database: failed to prune orphaned tools", "error", err)
	}

	return len(orphanedKeys), err
}
