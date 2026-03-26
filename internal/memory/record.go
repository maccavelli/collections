package memory

import (
	"encoding/json"
	"time"
)

// Record represents a single atomic entry in the memory store with metadata.
type Record struct {
	Content   string    `json:"content"`
	Category  string    `json:"category,omitempty"` // Primary classification
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SearchResult wraps a Record with its original key and an optional relevance score.
type SearchResult struct {
	Key         string  `json:"key"`
	Record      *Record `json:"record,omitempty"`
	Score       int     `json:"score,omitempty"`
	Summary     string  `json:"summary,omitempty"`
	IsTruncated bool    `json:"is_truncated,omitempty"`
}

// migrateRecord converts legacy string formats to the new Record struct if needed.
func migrateRecord(data []byte) (*Record, error) {
	var rec Record
	if err := json.Unmarshal(data, &rec); err == nil && rec.Content != "" {
		return &rec, nil
	}

	// Not valid JSON Record, assume legacy string
	return &Record{
		Content:   string(data),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Tags:      []string{"legacy"},
	}, nil
}
