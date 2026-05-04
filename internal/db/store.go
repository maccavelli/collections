// Package db provides BuntDB-backed session persistence for the MagicDev
// pipeline state machine.
package db

import (
	json "github.com/go-json-experiment/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/viper"
	"github.com/tidwall/buntdb"
)

// Store wraps a BuntDB instance for thread-safe session CRUD operations.
type Store struct {
	DB *buntdb.DB
}

// InitStore opens a BuntDB instance.
func InitStore() (*Store, error) {
	dbPath := viper.GetString("server.db_path")
	if dbPath == "" {
		dbPath = ":memory:"
	} else if dbPath != ":memory:" {
		dbPath = filepath.Clean(filepath.FromSlash(dbPath))
	}

	database, err := buntdb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return &Store{DB: database}, nil
}

// Close releases the underlying BuntDB resources.
func (s *Store) Close() error {
	return s.DB.Close()
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
