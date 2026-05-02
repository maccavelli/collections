package db

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tidwall/buntdb"
)

// Store encapsulates the BuntDB instance.
type Store struct {
	DB *buntdb.DB
}

// InitStore creates or opens the session database.
func InitStore() (*Store, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache dir: %w", err)
	}

	magicDir := filepath.Join(cacheDir, "magicdev")
	if err := os.MkdirAll(magicDir, 0700); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(magicDir, "session.db")
	db, err := buntdb.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open buntdb at %s: %w", dbPath, err)
	}

	return &Store{DB: db}, nil
}

// Close gracefully closes the database.
func (s *Store) Close() error {
	if s.DB != nil {
		return s.DB.Close()
	}
	return nil
}

func sessionKey(id string) string {
	return "session:" + id
}

// SaveSession serializes and stores a SessionState.
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

// LoadSession retrieves a SessionState by ID. Returns nil if not found.
func (s *Store) LoadSession(sessionID string) (*SessionState, error) {
	var session SessionState
	err := s.DB.View(func(tx *buntdb.Tx) error {
		val, err := tx.Get(sessionKey(sessionID))
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(val), &session)
	})

	if err == buntdb.ErrNotFound {
		return nil, nil // Return nil, nil when not found
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// AppendStandard appends a new rule/standard to an existing session.
func (s *Store) AppendStandard(sessionID, standard string) error {
	return s.DB.Update(func(tx *buntdb.Tx) error {
		val, err := tx.Get(sessionKey(sessionID))
		if err != nil {
			return err // Cannot append if session doesn't exist
		}

		var session SessionState
		if err := json.Unmarshal([]byte(val), &session); err != nil {
			return err
		}

		session.Standards = append(session.Standards, standard)
		
		data, err := json.Marshal(session)
		if err != nil {
			return err
		}

		_, _, err = tx.Set(sessionKey(sessionID), string(data), nil)
		return err
	})
}

// DeleteSession removes a session from the DB.
func (s *Store) DeleteSession(sessionID string) error {
	return s.DB.Update(func(tx *buntdb.Tx) error {
		_, err := tx.Delete(sessionKey(sessionID))
		return err
	})
}
