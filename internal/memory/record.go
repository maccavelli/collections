package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/klauspost/compress/zstd"
)

// Domain constants for namespace separation.
const (
	DomainMemories  = "memories"
	DomainStandards = "standards"
	DomainSessions  = "sessions"
	DomainProjects  = "projects"
)

var (
	zstdEncoder, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
	zstdDecoder, _ = zstd.NewReader(nil, zstd.WithDecoderConcurrency(0))
	zstdMagic      = []byte{0x28, 0xb5, 0x2f, 0xfd}
)

// Record represents a single atomic entry in the memory store with metadata.
type Record struct {
	Title      string    `json:"title,omitempty"`
	SymbolName string    `json:"symbolname,omitempty"`
	Content    string    `json:"content"`
	Category   string    `json:"category,omitempty"`   // Primary classification
	Domain     string    `json:"domain,omitempty"`     // Namespace: "memories" or "standards"
	SessionID  string    `json:"session_id,omitempty"` // Telemetry binding
	Tags       []string  `json:"tags,omitempty"`
	SourcePath string    `json:"source_path,omitempty"`
	SourceHash string    `json:"source_hash,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SearchResult wraps a Record with its original key and an optional relevance score.
type SearchResult struct {
	Key         string   `json:"key"`
	Record      *Record  `json:"record,omitempty"`
	Score       int      `json:"score,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	IsTruncated bool     `json:"is_truncated,omitempty"`
	Snippets    []string `json:"snippets,omitempty"`
}

// marshalRecord centralizes the serialization and Zstd compression of a Record.
func marshalRecord(rec *Record) ([]byte, error) {
	data, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	// Compress the JSON byte slice natively (returns compressed bytes with magic header)
	return zstdEncoder.EncodeAll(data, make([]byte, 0, len(data))), nil
}

// migrateRecord converts legacy string formats to the new Record struct if needed.
// Infers Domain from Category for backward compatibility with pre-domain records.
func migrateRecord(data []byte) (*Record, error) {
	return migrateRecordCtx(context.TODO(), data)
}

func migrateRecordCtx(ctx context.Context, data []byte) (*Record, error) {
	// Transparently handle Zstd-compressed records by sniffing the magic bytes
	if bytes.HasPrefix(data, zstdMagic) {
		var err error
		data, err = zstdDecoder.DecodeAll(data, nil)
		if err != nil {
			return nil, err
		}
	}

	var rec Record
	if err := json.Unmarshal(data, &rec); err == nil && rec.Content != "" {
		// Infer Domain for records written before the Domain field existed.
		if rec.Domain == "" {
			if HarvestedCategories[rec.Category] {
				rec.Domain = DomainStandards
			} else {
				rec.Domain = DomainMemories
			}
		}
		return &rec, nil
	}

	// Not valid JSON Record, assume legacy string
	return &Record{
		Content:   string(data),
		Domain:    DomainMemories,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Tags:      []string{"legacy"},
	}, nil
}
