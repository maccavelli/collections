// Package db provides BuntDB-backed session persistence for the MagicDev
// pipeline state machine.
package db

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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
	DB       *buntdb.DB
	FilePath string

	// Latency telemetry
	readOps     atomic.Uint64
	writeOps    atomic.Uint64
	readTimeUs  atomic.Uint64
	writeTimeUs atomic.Uint64
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
	return InitStoreWithPath(dbPath)
}

// InitStoreWithPath performs the InitStoreWithPath operation.
func InitStoreWithPath(dbPath string) (*Store, error) {
	if dbPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
			return nil, err
		}
	}

	database, err := buntdb.Open(dbPath)
	if err != nil {
		return nil, err
	}

	if dbPath != ":memory:" {
		if err := os.Chmod(dbPath, 0600); err != nil {
			database.Close()
			return nil, fmt.Errorf("failed to secure database file: %w", err)
		}
	}

	var bCfg buntdb.Config
	if err := database.ReadConfig(&bCfg); err == nil {
		bCfg.AutoShrinkPercentage = 50
		bCfg.AutoShrinkMinSize = 32 * 1024 * 1024
		bCfg.SyncPolicy = buntdb.EverySecond
		database.SetConfig(bCfg)
	}

	// Register baseline prefix index for efficient prefix iteration.
	if err := database.CreateIndex("idx_baselines", "baseline:*", buntdb.IndexString); err != nil {
		slog.Warn("failed to create baseline index", "error", err)
	}

	return &Store{DB: database, FilePath: dbPath}, nil
}

// Close releases the underlying BuntDB resources.
func (s *Store) Close() error {
	return s.DB.Close()
}

// DBSize returns the physical size of the BuntDB file in bytes.
func (s *Store) DBSize() int64 {
	if s == nil || s.FilePath == "" || s.FilePath == ":memory:" {
		return 0
	}
	info, err := os.Stat(s.FilePath)
	if err != nil {
		return 0
	}
	return info.Size()
}

// View wraps buntdb.View and tracks read latency.
func (s *Store) View(fn func(tx *buntdb.Tx) error) error {
	start := time.Now()
	err := s.DB.View(fn)
	s.readTimeUs.Add(uint64(time.Since(start).Microseconds()))
	s.readOps.Add(1)
	return err
}

// Update wraps buntdb.Update and tracks write latency.
func (s *Store) Update(fn func(tx *buntdb.Tx) error) error {
	start := time.Now()
	err := s.DB.Update(fn)
	s.writeTimeUs.Add(uint64(time.Since(start).Microseconds()))
	s.writeOps.Add(1)
	return err
}

// GetAndResetLatency returns average read and write latencies in microseconds,
// and resets the underlying counters.
func (s *Store) GetAndResetLatency() (avgReadUs, avgWriteUs, totalOps uint64) {
	ro := s.readOps.Swap(0)
	rt := s.readTimeUs.Swap(0)
	wo := s.writeOps.Swap(0)
	wt := s.writeTimeUs.Swap(0)

	if ro > 0 {
		avgReadUs = rt / ro
	}
	if wo > 0 {
		avgWriteUs = wt / wo
	}
	return avgReadUs, avgWriteUs, ro + wo
}

// DBEntries returns the total count of keys in the BuntDB store.
func (s *Store) DBEntries() int {
	if s == nil || s.DB == nil {
		return 0
	}
	var n int
	_ = s.View(func(tx *buntdb.Tx) error {
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
	if session.CurrentStep != "" {
		if session.StepDataBytes == nil {
			session.StepDataBytes = make(map[string]int)
		}
		tempData, _ := json.Marshal(session)
		session.StepDataBytes[session.CurrentStep] = len(tempData)
	}

	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(sessionKey(session.SessionID), string(data), nil)
		return err
	})
}

// SaveCompletedSession serializes and persists a completed session with a 7-day TTL.
// After the TTL expires, BuntDB automatically evicts the key.
func (s *Store) SaveCompletedSession(session *SessionState) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.Update(func(tx *buntdb.Tx) error {
		opts := &buntdb.SetOptions{Expires: true, TTL: 7 * 24 * time.Hour}
		_, _, err := tx.Set(sessionKey(session.SessionID), string(data), opts)
		if err != nil {
			return err
		}
		// Also set TTL on the corresponding metadata key if it exists.
		if _, metaErr := tx.Get(sessionMetadataKey(session.SessionID)); metaErr == nil {
			// Re-set with the same TTL so both expire together.
			val, _ := tx.Get(sessionMetadataKey(session.SessionID))
			_, _, _ = tx.Set(sessionMetadataKey(session.SessionID), val, opts)
		}
		return nil
	})
}

// LoadSession retrieves and deserializes a session by ID.
// Returns (nil, nil) if the session does not exist.
func (s *Store) LoadSession(sessionID string) (*SessionState, error) {
	var session SessionState
	err := s.View(func(tx *buntdb.Tx) error {
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

// ListSessions returns all active session states sorted chronologically by CreatedAt.
func (s *Store) ListSessions() ([]SessionState, error) {
	var sessions []SessionState
	err := s.View(func(tx *buntdb.Tx) error {
		tx.AscendKeys("session:*", func(key, val string) bool {
			if !strings.HasPrefix(key, "session_metadata:") {
				var session SessionState
				if err := json.Unmarshal([]byte(val), &session); err == nil {
					sessions = append(sessions, session)
				}
			}
			return true
		})
		return nil
	})
	
	// Sort chronologically (UUID keys are not chronological)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt < sessions[j].CreatedAt
	})
	
	return sessions, err
}

// sessionMetadataKey returns the canonical BuntDB key for session metadata.
func sessionMetadataKey(id string) string {
	return fmt.Sprintf("session_metadata:%s", id)
}

// GetSessionMetadata retrieves session metadata.
func (s *Store) GetSessionMetadata(sessionID string) (*SessionMetadata, error) {
	var meta SessionMetadata
	err := s.View(func(tx *buntdb.Tx) error {
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
	return s.Update(func(tx *buntdb.Tx) error {
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

// DeleteSession removes a session and its associated metadata from the DB.
func (s *Store) DeleteSession(sessionID string) error {
	return s.Update(func(tx *buntdb.Tx) error {
		_, err := tx.Delete(sessionKey(sessionID))
		if err != nil {
			return err
		}
		// Best-effort metadata cleanup — ignore ErrNotFound.
		if _, metaErr := tx.Delete(sessionMetadataKey(sessionID)); metaErr != nil {
			if !errors.Is(metaErr, buntdb.ErrNotFound) {
				return metaErr
			}
		}
		return nil
	})
}

// PurgeSessions deletes all session:* and session_metadata:* keys atomically,
// leaving baseline:* and secret:* keys intact. Returns the count of deleted keys.
func (s *Store) PurgeSessions() (int, error) {
	var count int
	err := s.Update(func(tx *buntdb.Tx) error {
		var keys []string
		tx.AscendKeys("session:*", func(key, _ string) bool {
			keys = append(keys, key)
			return true
		})
		tx.AscendKeys("session_metadata:*", func(key, _ string) bool {
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
	err := s.Update(func(tx *buntdb.Tx) error {
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

// PurgeChaosGraveyards deletes all chaos:graveyard:* keys atomically. Returns the count of deleted keys.
func (s *Store) PurgeChaosGraveyards() (int, error) {
	var count int
	err := s.Update(func(tx *buntdb.Tx) error {
		var keys []string
		tx.AscendKeys("chaos:graveyard:*", func(key, _ string) bool {
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
	return s.Update(func(tx *buntdb.Tx) error {
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
	err := s.View(func(tx *buntdb.Tx) error {
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
	err := s.View(func(tx *buntdb.Tx) error {
		_, err := tx.Get(baselineKey(url))
		return err
	})
	return err == nil
}

// GetBaselineContent retrieves and decompresses a baseline standard.
func (s *Store) GetBaselineContent(url string) (string, error) {
	var standard BaselineStandard
	err := s.View(func(tx *buntdb.Tx) error {
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
	err := s.View(func(tx *buntdb.Tx) error {
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
	_ = s.View(func(tx *buntdb.Tx) error {
		tx.AscendKeys("baseline:*", func(_, _ string) bool {
			count++
			return true
		})
		return nil
	})
	return count
}

// SessionCount returns the number of cached session states.
func (s *Store) SessionCount() int {
	var count int
	_ = s.View(func(tx *buntdb.Tx) error {
		tx.AscendKeys("session:*", func(key, _ string) bool {
			if !strings.HasPrefix(key, "session_metadata:") {
				count++
			}
			return true
		})
		return nil
	})
	return count
}

// ChaosGraveyardCount returns the number of cached chaos rejection patterns.
func (s *Store) ChaosGraveyardCount() int {
	var count int
	_ = s.View(func(tx *buntdb.Tx) error {
		tx.AscendKeys("chaos:graveyard:*", func(_, val string) bool {
			var patterns []ChaosRejection
			if err := json.Unmarshal([]byte(val), &patterns); err == nil {
				count += len(patterns)
			}
			return true
		})
		return nil
	})
	return count
}

// ListAllChaosGraveyards returns all chaos rejection patterns across all stacks
// for dashboard anti-pattern frequency visualization.
func (s *Store) ListAllChaosGraveyards() []ChaosRejection {
	var all []ChaosRejection
	_ = s.View(func(tx *buntdb.Tx) error {
		tx.AscendKeys("chaos:graveyard:*", func(_, val string) bool {
			var patterns []ChaosRejection
			if json.Unmarshal([]byte(val), &patterns) == nil {
				all = append(all, patterns...)
			}
			return true
		})
		return nil
	})
	return all
}

func chaosGraveyardKey(stack string) string {
	return fmt.Sprintf("chaos:graveyard:%s", stack)
}

// SaveChaosGraveyard stores or appends rejected patterns to the graveyard for a given stack.
func (s *Store) SaveChaosGraveyard(stack string, patterns []ChaosRejection) error {
	if len(patterns) == 0 {
		return nil
	}

	return s.Update(func(tx *buntdb.Tx) error {
		key := chaosGraveyardKey(stack)
		var existing []ChaosRejection

		val, err := tx.Get(key)
		if err == nil {
			json.Unmarshal([]byte(val), &existing)
		}

		// Deduplicate using text matching
		seen := make(map[string]bool)
		for _, p := range existing {
			seen[p.Pattern] = true
		}

		for _, p := range patterns {
			if !seen[p.Pattern] {
				existing = append(existing, p)
				seen[p.Pattern] = true
			}
		}

		data, err := json.Marshal(existing)
		if err != nil {
			return err
		}

		// 90 day TTL
		opts := &buntdb.SetOptions{Expires: true, TTL: 90 * 24 * time.Hour}
		_, _, err = tx.Set(key, string(data), opts)
		return err
	})
}

// GetChaosGraveyard retrieves the chaos graveyard for a given stack.
func (s *Store) GetChaosGraveyard(stack string) ([]ChaosRejection, error) {
	var existing []ChaosRejection
	err := s.View(func(tx *buntdb.Tx) error {
		val, err := tx.Get(chaosGraveyardKey(stack))
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(val), &existing)
	})

	if err != nil {
		if errors.Is(err, buntdb.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return existing, nil
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

	return s.Update(func(tx *buntdb.Tx) error {
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

	return s.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(secretKey(service), encryptedToken, nil)
		return err
	})
}

// GetSecret retrieves and decrypts the token for the given service.
// Returns an empty string if the secret does not exist.
func (s *Store) GetSecret(service string) (string, error) {
	var encryptedToken string
	err := s.View(func(tx *buntdb.Tx) error {
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
