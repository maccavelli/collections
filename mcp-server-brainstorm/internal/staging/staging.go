// Package staging provides functionality for the staging subsystem.
package staging

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/buntdb"
)

// Wrapper provides a TTL-enabled shell for JSON payloads.
type Wrapper struct {
	ExpiresAt time.Time `json:"expires_at"`
	Data      any       `json:"data"`
}

// SetupIndexes configures TTL spatial indexes for the staging area.
func SetupIndexes(db *buntdb.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	// We use buntdb.IndexJSON to index the expires_at field natively
	return db.CreateIndex("expires_at", "staging:*", buntdb.IndexJSON("expires_at"))
}

// SavePayload serializes data into a JSON wrapper with a 2-hour TTL
// and writes it to BuntDB. Returns the staging URI (e.g. "staging:uuid").
func SavePayload(db *buntdb.DB, data any) (string, error) {
	if db == nil {
		return "", fmt.Errorf("db is nil")
	}

	id := uuid.New().String()
	uri := fmt.Sprintf("staging:%s", id)

	wrapper := Wrapper{
		ExpiresAt: time.Now().Add(2 * time.Hour),
		Data:      data,
	}

	payloadBytes, err := json.Marshal(wrapper)
	if err != nil {
		return "", fmt.Errorf("failed to marshal staging wrapper: %w", err)
	}

	err = db.Update(func(tx *buntdb.Tx) error {
		opts := &buntdb.SetOptions{Expires: true, TTL: 2 * time.Hour}
		_, _, err := tx.Set(uri, string(payloadBytes), opts)
		return err
	})

	if err != nil {
		return "", fmt.Errorf("failed to write staging payload: %w", err)
	}

	return uri, nil
}

// LoadPayload retrieves and unmarshals a staged payload by URI into the provided target pointer.
func LoadPayload(db *buntdb.DB, uri string, target any) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if !strings.HasPrefix(uri, "staging:") {
		return fmt.Errorf("invalid staging uri format")
	}

	var raw string
	err := db.View(func(tx *buntdb.Tx) error {
		val, err := tx.Get(uri)
		if err != nil {
			return err
		}
		raw = val
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to read staging payload (may be expired): %w", err)
	}

	var wrapper Wrapper
	wrapper.Data = target // Inject target pointer to allow direct decoding
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return fmt.Errorf("failed to unmarshal staging wrapper: %w", err)
	}

	return nil
}

// SaveSocraticVerdict saves a Socratic evaluation verdict directly to BuntDB for later report generation.
func SaveSocraticVerdict(db *buntdb.DB, sessionID, toolName, verdict string, details any) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if sessionID == "" || toolName == "" {
		return fmt.Errorf("sessionID and toolName are required")
	}

	uri := fmt.Sprintf("verdict:%s:%s", sessionID, toolName)
	wrapper := Wrapper{
		ExpiresAt: time.Now().Add(24 * time.Hour), // 24-hour retention for pipeline reports
		Data: map[string]any{
			"tool":    toolName,
			"verdict": verdict,
			"details": details,
		},
	}

	payloadBytes, err := json.Marshal(wrapper)
	if err != nil {
		return fmt.Errorf("failed to marshal verdict wrapper: %w", err)
	}

	return db.Update(func(tx *buntdb.Tx) error {
		opts := &buntdb.SetOptions{Expires: true, TTL: 24 * time.Hour}
		_, _, err := tx.Set(uri, string(payloadBytes), opts)
		return err
	})
}

// LoadSocraticVerdicts retrieves all Socratic verdicts saved for a given session.
func LoadSocraticVerdicts(db *buntdb.DB, sessionID string) ([]map[string]any, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}

	var results []map[string]any
	pattern := fmt.Sprintf("verdict:%s:*", sessionID)

	err := db.View(func(tx *buntdb.Tx) error {
		return tx.AscendKeys(pattern, func(key, value string) bool {
			var wrapper Wrapper
			// We only want the Data portion as map[string]any
			var data map[string]any
			wrapper.Data = &data
			if err := json.Unmarshal([]byte(value), &wrapper); err == nil {
				results = append(results, data)
			}
			return true // continue iteration
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve verdicts: %w", err)
	}

	return results, nil
}

// IsStagingURI returns true if the string is a valid staging pointer.
func IsStagingURI(s string) bool {
	return strings.HasPrefix(s, "staging:")
}
