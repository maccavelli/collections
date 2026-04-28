//go:build windows

package vector

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"mcp-server-magictools/internal/config"
)

// Engine is the central abstraction for semantic intelligence inside magictools.
type Engine struct {
	mu          sync.RWMutex
	embedder    Embedder
	dbPath      string
	metaPath    string
	initialized bool
}

var (
	GlobalEngine *Engine
	initOnce     sync.Once
)

// InitGlobalEngine initializes the global singleton for Windows (fallback mode).
func InitGlobalEngine(dbDir string, cfg *config.Config) error {
	var err error
	initOnce.Do(func() {
		slog.Warn("vector engine boot: Windows architecture detected. HNSW is currently unsupported on Windows. Falling back to BM25 natively.",
			"component", "vector",
			"os", "windows")

		GlobalEngine = &Engine{
			embedder:    nil,
			initialized: true,
		}
	})
	return err
}

// VectorEnabled returns true if the engine has a valid embedder configured.
func (e *Engine) VectorEnabled() bool {
	return false
}

// GetEngine returns the global engine safely.
func GetEngine() *Engine {
	return GlobalEngine
}

// Save serializes the HNSW graph to disk.
func (e *Engine) Save() error {
	return nil
}

// AddDocument embeds text and inserts the vector into the HNSW index.
func (e *Engine) AddDocument(ctx context.Context, id string, text string) error {
	return fmt.Errorf("cannot add document: Vector capability is offline on Windows")
}

// HasDocument returns true if the URN natively exists within the active HNSW graph graph.
func (e *Engine) HasDocument(id string) bool {
	return false
}

// RequiresHydration returns true if the native HNSW graph is mathematically empty and requires a structural injection.
func (e *Engine) RequiresHydration() bool {
	return false
}

// Len returns the number of nodes currently in the HNSW graph.
func (e *Engine) Len() int {
	return 0
}

// Search queries the HNSW index for nearest neighbors to the intent.
func (e *Engine) Search(ctx context.Context, intent string, k int) ([]string, error) {
	return nil, fmt.Errorf("cannot search: Vector capability is offline on Windows")
}

// ScoredResult pairs a URN key with its actual cosine similarity score.
type ScoredResult struct {
	Key   string
	Score float64
}

// SearchWithScores queries the HNSW index and returns actual cosine similarity scores.
func (e *Engine) SearchWithScores(ctx context.Context, intent string, k int) ([]ScoredResult, error) {
	return nil, fmt.Errorf("cannot search: Vector capability is offline on Windows")
}

// SearchByNode looks up a pre-embedded URN natively within the HNSW graph and computes actual Cosine Distances.
func (e *Engine) SearchByNode(ctx context.Context, urn string, k int) ([]ScoredResult, error) {
	return nil, fmt.Errorf("cannot search: Vector capability is offline on Windows")
}
