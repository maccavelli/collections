// Package db provides BuntDB-backed session persistence for the MagicDev
// pipeline state machine.
package db

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	json "github.com/go-json-experiment/json"
	"github.com/klauspost/compress/zstd"
	"github.com/spf13/viper"
	"github.com/tidwall/buntdb"
	"mcp-server-magicdev/internal/vault"
)

// Pooled zstd encoder and decoder to avoid per-call allocation overhead.
// The klauspost/compress library is designed for reuse — EncodeAll/DecodeAll
// are stateless and safe to call on pooled instances without Reset().
// Do NOT call Close() on pooled objects.
var zstdEncoderPool = sync.Pool{
	New: func() any {
		enc, _ := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
		return enc
	},
}

var zstdDecoderPool = sync.Pool{
	New: func() any {
		dec, _ := zstd.NewReader(nil, zstd.WithDecoderConcurrency(0))
		return dec
	},
}

// Store wraps a BuntDB instance for thread-safe session CRUD operations.
type Store struct {
	DB *buntdb.DB
}

// BaselineStandard represents a Zstd-compressed hybrid Markdown standard.
type BaselineStandard struct {
	Hash    string `json:"hash"`
	Payload []byte `json:"payload"` // Zstd compressed markdown
}

// BaselineMeta contains lightweight metadata about a cached standard
// without the compressed payload.
type BaselineMeta struct {
	URL  string `json:"url"`
	Hash string `json:"hash"`
}

// InitStore opens a BuntDB instance.
func InitStore() (*Store, error) {
	dbPath := viper.GetString("server.db_path")
	if dbPath == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			cacheDir = os.TempDir()
		}
		dbPath = filepath.Join(cacheDir, "mcp-server-magicdev", "session.db")
	} else if dbPath != ":memory:" {
		dbPath = filepath.Clean(filepath.FromSlash(dbPath))
	}

	if dbPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			return nil, err
		}
	}

	database, err := buntdb.Open(dbPath)
	if err != nil {
		return nil, err
	}

	var bCfg buntdb.Config
	if err := database.ReadConfig(&bCfg); err == nil {
		bCfg.AutoShrinkPercentage = 50
		bCfg.AutoShrinkMinSize = 32 * 1024 * 1024
		bCfg.SyncPolicy = buntdb.Never
		database.SetConfig(bCfg)
	}

	// Register baseline prefix index for efficient prefix iteration.
	if err := database.CreateIndex("idx_baselines", "baseline:*", buntdb.IndexString); err != nil {
		slog.Warn("failed to create baseline index", "error", err)
	}

	return &Store{DB: database}, nil
}

// Close releases the underlying BuntDB resources.
func (s *Store) Close() error {
	return s.DB.Close()
}

// DBEntries returns the total count of keys in the BuntDB store.
func (s *Store) DBEntries() int {
	if s == nil || s.DB == nil {
		return 0
	}
	var n int
	_ = s.DB.View(func(tx *buntdb.Tx) error {
		n, _ = tx.Len()
		return nil
	})
	return n
}

// sessionKey returns the canonical BuntDB key for a session.
func sessionKey(id string) string {
	return fmt.Sprintf("session:%s", id)
}

// SaveSession serializes and persists a complete session state.
func (s *Store) SaveSession(session *SessionState) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.DB.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(sessionKey(session.SessionID), string(data), nil)
		return err
	})
}

// LoadSession retrieves and deserializes a session by ID.
// Returns (nil, nil) if the session does not exist.
func (s *Store) LoadSession(sessionID string) (*SessionState, error) {
	var session SessionState
	err := s.DB.View(func(tx *buntdb.Tx) error {
		val, err := tx.Get(sessionKey(sessionID))
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(val), &session)
	})
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// sessionMetadataKey returns the canonical BuntDB key for session metadata.
func sessionMetadataKey(id string) string {
	return fmt.Sprintf("session_metadata:%s", id)
}

// GetSessionMetadata retrieves session metadata.
func (s *Store) GetSessionMetadata(sessionID string) (*SessionMetadata, error) {
	var meta SessionMetadata
	err := s.DB.View(func(tx *buntdb.Tx) error {
		val, err := tx.Get(sessionMetadataKey(sessionID))
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(val), &meta)
	})
	if err != nil {
		if errors.Is(err, buntdb.ErrNotFound) {
			return &SessionMetadata{SessionID: sessionID}, nil
		}
		return nil, err
	}
	return &meta, nil
}

// SaveSessionMetadata saves session metadata.
func (s *Store) SaveSessionMetadata(meta *SessionMetadata) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return s.DB.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(sessionMetadataKey(meta.SessionID), string(data), nil)
		return err
	})
}

// AppendStandard adds a standard to the session's Standards slice within
// a single BuntDB transaction to prevent interleaving writes.
func (s *Store) AppendStandard(sessionID, standard string) error {
	return s.updateSession(sessionID, func(session *SessionState) {
		session.Standards = append(session.Standards, standard)
	})
}

// SaveBlueprint stores a Blueprint into an existing session within
// a single transaction.
func (s *Store) SaveBlueprint(sessionID string, bp *Blueprint) error {
	return s.updateSession(sessionID, func(session *SessionState) {
		session.Blueprint = bp
	})
}

// UpdateCurrentStep sets the CurrentStep field on an existing session.
func (s *Store) UpdateCurrentStep(sessionID, step string) error {
	return s.updateSession(sessionID, func(session *SessionState) {
		session.CurrentStep = step
	})
}

// DeleteSession removes a session from the DB.
func (s *Store) DeleteSession(sessionID string) error {
	return s.DB.Update(func(tx *buntdb.Tx) error {
		_, err := tx.Delete(sessionKey(sessionID))
		return err
	})
}

// PurgeSessions deletes all session:* keys atomically, leaving
// baseline:* and secret:* keys intact. Returns the count of deleted keys.
func (s *Store) PurgeSessions() (int, error) {
	var count int
	err := s.DB.Update(func(tx *buntdb.Tx) error {
		var keys []string
		tx.AscendKeys("session:*", func(key, _ string) bool {
			keys = append(keys, key)
			return true
		})
		for _, key := range keys {
			if _, err := tx.Delete(key); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

// PurgeBaselines deletes all baseline:* keys atomically, leaving
// session:* and secret:* keys intact. Returns the count of deleted keys.
func (s *Store) PurgeBaselines() (int, error) {
	var count int
	err := s.DB.Update(func(tx *buntdb.Tx) error {
		var keys []string
		tx.AscendKeys("baseline:*", func(key, _ string) bool {
			keys = append(keys, key)
			return true
		})
		for _, key := range keys {
			if _, err := tx.Delete(key); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

// AppendStepStatus atomically updates a single step's status within a session.
func (s *Store) AppendStepStatus(sessionID, step, status string) error {
	return s.updateSession(sessionID, func(session *SessionState) {
		session.StepStatus[step] = status
	})
}

// updateSession is a generic read-modify-write helper that eliminates the
// duplicated unmarshal/modify/marshal pattern across all mutating operations.
func (s *Store) updateSession(sessionID string, mutate func(*SessionState)) error {
	return s.DB.Update(func(tx *buntdb.Tx) error {
		val, err := tx.Get(sessionKey(sessionID))
		if err != nil {
			return err
		}

		var session SessionState
		if err := json.Unmarshal([]byte(val), &session); err != nil {
			return err
		}

		mutate(&session)

		data, err := json.Marshal(session)
		if err != nil {
			return err
		}

		_, _, err = tx.Set(sessionKey(sessionID), string(data), nil)
		return err
	})
}

// baselineKey returns the canonical BuntDB key for a baseline standard.
func baselineKey(url string) string {
	return fmt.Sprintf("baseline:%s", url)
}

// GetBaselineHash fetches only the hash for a stored baseline standard.
func (s *Store) GetBaselineHash(url string) (string, error) {
	var standard BaselineStandard
	err := s.DB.View(func(tx *buntdb.Tx) error {
		val, err := tx.Get(baselineKey(url))
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(val), &standard)
	})
	if err != nil {
		return "", err
	}
	return standard.Hash, nil
}

// HasBaseline returns true if the baseline key exists and has not expired.
// This is a zero-decompression, zero-unmarshal check — just a BuntDB key probe.
// BuntDB handles TTL expiration internally: expired keys return ErrNotFound on Get.
func (s *Store) HasBaseline(url string) bool {
	err := s.DB.View(func(tx *buntdb.Tx) error {
		_, err := tx.Get(baselineKey(url))
		return err
	})
	return err == nil
}

// GetBaselineContent retrieves and decompresses a baseline standard.
func (s *Store) GetBaselineContent(url string) (string, error) {
	var standard BaselineStandard
	err := s.DB.View(func(tx *buntdb.Tx) error {
		val, err := tx.Get(baselineKey(url))
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(val), &standard)
	})
	if err != nil {
		return "", err
	}

	decoder := zstdDecoderPool.Get().(*zstd.Decoder)
	defer zstdDecoderPool.Put(decoder)

	uncompressed, err := decoder.DecodeAll(standard.Payload, nil)
	if err != nil {
		return "", err
	}

	return string(uncompressed), nil
}

// ListBaselines returns metadata for all cached baseline standards
// without decompressing payloads. Uses the idx_baselines index for
// efficient prefix iteration.
func (s *Store) ListBaselines() ([]BaselineMeta, error) {
	var metas []BaselineMeta
	err := s.DB.View(func(tx *buntdb.Tx) error {
		tx.AscendKeys("baseline:*", func(key, val string) bool {
			var std BaselineStandard
			if json.Unmarshal([]byte(val), &std) == nil {
				metas = append(metas, BaselineMeta{
					URL:  strings.TrimPrefix(key, "baseline:"),
					Hash: std.Hash,
				})
			}
			return true
		})
		return nil
	})
	return metas, err
}

// BaselineCount returns the number of cached baseline standards.
func (s *Store) BaselineCount() int {
	var count int
	_ = s.DB.View(func(tx *buntdb.Tx) error {
		tx.AscendKeys("baseline:*", func(_, _ string) bool {
			count++
			return true
		})
		return nil
	})
	return count
}

// SetBaseline compresses and stores a baseline standard in BuntDB.
func (s *Store) SetBaseline(url string, content string, hash string) error {
	encoder := zstdEncoderPool.Get().(*zstd.Encoder)
	defer zstdEncoderPool.Put(encoder)

	compressed := encoder.EncodeAll([]byte(content), make([]byte, 0, len(content)))

	standard := BaselineStandard{
		Hash:    hash,
		Payload: compressed,
	}

	data, err := json.Marshal(standard)
	if err != nil {
		return err
	}

	return s.DB.Update(func(tx *buntdb.Tx) error {
		opts := &buntdb.SetOptions{Expires: true, TTL: 30 * 24 * time.Hour}
		_, _, err := tx.Set(baselineKey(url), string(data), opts)
		return err
	})
}

// secretKey returns the canonical BuntDB key for a service token/secret.
func secretKey(service string) string {
	return fmt.Sprintf("secret:%s", service)
}

// SetSecret encrypts the given token and stores it in BuntDB.
func (s *Store) SetSecret(service, token string) error {
	encryptedToken, err := vault.Encrypt(token)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret for %s: %w", service, err)
	}

	return s.DB.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(secretKey(service), encryptedToken, nil)
		return err
	})
}

// GetSecret retrieves and decrypts the token for the given service.
// Returns an empty string if the secret does not exist.
func (s *Store) GetSecret(service string) (string, error) {
	var encryptedToken string
	err := s.DB.View(func(tx *buntdb.Tx) error {
		val, err := tx.Get(secretKey(service))
		if err != nil {
			return err
		}
		encryptedToken = val
		return nil
	})

	if err != nil {
		if errors.Is(err, buntdb.ErrNotFound) {
			return "", nil
		}
		return "", err
	}

	decryptedToken, err := vault.Decrypt(encryptedToken)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt secret for %s: %w", service, err)
	}

	return decryptedToken, nil
}
