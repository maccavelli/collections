//go:build !windows

package vector

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/coder/hnsw"

	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/telemetry"
)

// Engine is the central abstraction for semantic intelligence inside magictools.
type Engine struct {
	mu          sync.RWMutex
	graph       *hnsw.Graph[string]
	embedder    Embedder
	dbPath      string
	metaPath    string
	initialized bool
}

var (
	GlobalEngine *Engine
	initOnce     sync.Once
)

// dimensionSentinel stores the embedding configuration hash to detect config changes.
type dimensionSentinel struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Dims     int    `json:"dims"`
	APIURL   string `json:"api_url,omitempty"`
	Hash     string `json:"hash"`
}

func computeSentinelHash(provider, model string, dims int, apiURL string) string {
	data := fmt.Sprintf("%s:%s:%d:%s", provider, model, dims, apiURL)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
}

// createHNSWGraph constructs an HNSW graph properly initialized with
// the required Distance function and configuration parameters.
func createHNSWGraph() *hnsw.Graph[string] {
	g := hnsw.NewGraph[string]()
	g.Distance = hnsw.CosineDistance
	g.M = 16
	g.EfSearch = 20
	// Ml defaults to 1/ln(M) inside the library if left 0, which is correct
	return g
}

// InitGlobalEngine initializes the global singleton for the HNSW index.
// It reads embedding configuration from the Config struct.
func InitGlobalEngine(dbDir string, cfg *config.Config) error {
	var err error
	initOnce.Do(func() {
		slog.Info("vector engine boot: STAGE 1 — resolving embedding configuration",
			"component", "vector",
			"embedding_provider", cfg.Intelligence.EmbeddingProvider,
			"embedding_model", cfg.Intelligence.EmbeddingModel,
			"embedded_dimensionality", cfg.Intelligence.EmbeddedDimensionality)

		embedder := NewEmbedderFromConfig(cfg)

		if embedder == nil {
			slog.Warn("vector engine boot: DISABLED — no valid embedder could be constructed. falling back to BM25.",
				"component", "vector")
		} else {
			slog.Info("vector engine boot: STAGE 2 — embedder constructed successfully",
				"component", "vector",
				"provider", embedder.Provider())
		}

		dbPath := filepath.Join(dbDir, "magictools_vector.blob")
		metaPath := filepath.Join(dbDir, "magictools_vector.meta")

		// 🛡️ DIMENSION SENTINEL: Check if config changed since last boot
		needsRebuild := false
		if embedder != nil {
			currentHash := computeSentinelHash(
				cfg.Intelligence.EmbeddingProvider,
				cfg.Intelligence.EmbeddingModel,
				cfg.Intelligence.EmbeddedDimensionality,
				cfg.Intelligence.EmbeddingAPIURL,
			)

			slog.Info("vector engine boot: STAGE 3 — checking dimension sentinel",
				"component", "vector",
				"sentinel_hash", currentHash[:12]+"...",
				"meta_path", metaPath)

			if metaData, readErr := os.ReadFile(metaPath); readErr == nil {
				var stored dimensionSentinel
				if json.Unmarshal(metaData, &stored) == nil && stored.Hash != currentHash {
					slog.Warn("vector engine boot: SENTINEL MISMATCH — wiping HNSW graph for full rebuild",
						"component", "vector",
						"old_provider", stored.Provider,
						"new_provider", cfg.Intelligence.EmbeddingProvider,
						"old_model", stored.Model,
						"new_model", cfg.Intelligence.EmbeddingModel,
						"old_dims", stored.Dims,
						"new_dims", cfg.Intelligence.EmbeddedDimensionality)
					needsRebuild = true
					_ = os.Remove(dbPath)
				} else if stored.Hash == currentHash {
					slog.Info("vector engine boot: SENTINEL MATCH — config unchanged, reusing existing graph",
						"component", "vector",
						"provider", stored.Provider,
						"model", stored.Model,
						"dims", stored.Dims)
				}
			} else {
				slog.Warn("vector engine boot: SENTINEL MISSING — first boot or corrupted meta, wiping stale graph",
					"component", "vector",
					"error", readErr)
				needsRebuild = true
				_ = os.Remove(dbPath)
			}

			// Write current sentinel
			sentinel := dimensionSentinel{
				Provider: cfg.Intelligence.EmbeddingProvider,
				Model:    cfg.Intelligence.EmbeddingModel,
				Dims:     cfg.Intelligence.EmbeddedDimensionality,
				APIURL:   cfg.Intelligence.EmbeddingAPIURL,
				Hash:     currentHash,
			}
			if data, mErr := json.MarshalIndent(sentinel, "", "  "); mErr == nil {
				_ = os.MkdirAll(filepath.Dir(metaPath), 0755)
				_ = os.WriteFile(metaPath, data, 0644)
				slog.Debug("vector engine boot: sentinel file written",
					"component", "vector", "path", metaPath)
			}
		}

		slog.Info("vector engine boot: STAGE 4 — loading HNSW graph",
			"component", "vector",
			"needs_rebuild", needsRebuild,
			"blob_path", dbPath)

		var graph *hnsw.Graph[string]
		if !needsRebuild {
			f, ferr := os.Open(dbPath)
			if ferr == nil {
				defer f.Close()
				graph = createHNSWGraph()

				func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Warn("vector engine boot: HNSW graph import panic intercepted, rebuilding index seamlessly.", "component", "vector", "panic", r)
							graph = createHNSWGraph()
							_ = os.Remove(dbPath)
						}
					}()

					if ierr := graph.Import(bufio.NewReader(f)); ierr != nil {
						slog.Warn("vector engine boot: HNSW graph import failed, starting fresh",
							"component", "vector", "error", ierr)
						graph = createHNSWGraph()
					} else {
						// 🛡️ Ensure distance func is attached even after import
						graph.Distance = hnsw.CosineDistance
						slog.Info("vector engine boot: HNSW graph imported successfully from disk",
							"component", "vector")
					}
				}()
			} else {
				slog.Info("vector engine boot: no existing graph file found, creating empty graph",
					"component", "vector")
				graph = createHNSWGraph()
			}
		} else {
			slog.Info("vector engine boot: creating fresh HNSW graph (rebuild required)",
				"component", "vector")
			graph = createHNSWGraph()
		}

		if embedder != nil && cfg.Intelligence.EmbeddingModel != "" {
			if contains(cfg.Intelligence.EmbeddingModel, "preview") {
				slog.Warn("vector engine boot: WARNING — embedding model contains 'preview' tag, may be deprecated without notice",
					"component", "vector",
					"model", cfg.Intelligence.EmbeddingModel)
			}
		}

		GlobalEngine = &Engine{
			graph:       graph,
			embedder:    embedder,
			dbPath:      dbPath,
			metaPath:    metaPath,
			initialized: true,
		}

		if embedder != nil {
			slog.Info("vector engine boot: STAGE 5 — READY ✓ semantic search ONLINE",
				"component", "vector",
				"provider", embedder.Provider(),
				"model", cfg.Intelligence.EmbeddingModel,
				"dims", cfg.Intelligence.EmbeddedDimensionality,
				"needs_hydration", needsRebuild)
		} else {
			slog.Info("vector engine boot: STAGE 5 — READY ✓ semantic search OFFLINE (BM25 fallback active)",
				"component", "vector")
		}
	})
	return err
}

// VectorEnabled returns true if the engine has a valid embedder configured.
func (e *Engine) VectorEnabled() bool {
	if e == nil {
		return false
	}
	return e.embedder != nil && e.graph != nil
}

// GetEngine returns the global engine safely.
func GetEngine() *Engine {
	return GlobalEngine
}

// Save serializes the HNSW graph to disk.
func (e *Engine) Save() error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	f, err := os.Create(e.dbPath)
	if err != nil {
		return fmt.Errorf("failed to create vector db backup: %w", err)
	}
	defer f.Close()

	if err := e.graph.Export(f); err != nil {
		return fmt.Errorf("failed to export hnsw graph: %w", err)
	}
	return nil
}

// AddDocument embeds text and inserts the vector into the HNSW index.
// For Gemini, this uses RETRIEVAL_DOCUMENT task type for asymmetric search.
func (e *Engine) AddDocument(ctx context.Context, id string, text string) error {
	if !e.VectorEnabled() {
		return fmt.Errorf("cannot add document: Vector capability is offline")
	}

	// 🛡️ NATIVE LOOKUP GUARD: Skip if already embedded, preventing panics and saving API tokens
	e.mu.RLock()
	_, exists := e.graph.Lookup(id)
	e.mu.RUnlock()
	if exists {
		return nil
	}

	// 🛡️ ASYMMETRIC EMBEDDING: Mark as document for indexing
	docCtx := WithTaskType(ctx, "RETRIEVAL_DOCUMENT")

	vector, err := e.embedder.Embed(docCtx, text)
	if err != nil {
		return err
	}

	// 🛡️ NIL VECTOR GUARD: Prevent HNSW CosineDistance panic on nil/empty embedding.
	// The LLM API can return success with an empty embedding payload.
	if len(vector) == 0 {
		return fmt.Errorf("embedding returned nil/empty vector for %q", id)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// 🛡️ DEFENSIVE: Belt-and-suspenders nil graph check.
	if e.graph == nil {
		return fmt.Errorf("HNSW graph is nil, cannot add document %q", id)
	}

	e.graph.Add(hnsw.MakeNode(id, vector))
	return nil
}

// HasDocument returns true if the URN natively exists within the active HNSW graph graph.
func (e *Engine) HasDocument(id string) bool {
	if !e.VectorEnabled() || e.graph == nil {
		return false
	}
	e.mu.RLock()
	_, exists := e.graph.Lookup(id)
	e.mu.RUnlock()
	return exists
}

// RequiresHydration returns true if the native HNSW graph is mathematically empty and requires a structural injection.
func (e *Engine) RequiresHydration() bool {
	if !e.VectorEnabled() || e.graph == nil {
		return false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.graph.Len() == 0
}

// Len returns the number of nodes currently in the HNSW graph.
func (e *Engine) Len() int {
	if !e.VectorEnabled() || e.graph == nil {
		return 0
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.graph.Len()
}

// Search queries the HNSW index for nearest neighbors to the intent.
// For Gemini, this uses RETRIEVAL_QUERY task type for asymmetric search.
func (e *Engine) Search(ctx context.Context, intent string, k int) ([]string, error) {
	if !e.VectorEnabled() {
		return nil, fmt.Errorf("cannot search: Vector capability is offline")
	}

	// 🛡️ ASYMMETRIC EMBEDDING: Mark as query for searching
	queryCtx := WithTaskType(ctx, "RETRIEVAL_QUERY")

	targetVec, err := e.embedder.Embed(queryCtx, intent)
	if err != nil {
		return nil, err
	}

	e.mu.RLock()
	var nodes []hnsw.Node[string]

	// 🛡️ NIL DEREF GUARD: The `github.com/coder/hnsw` library has a critical native fault
	// where it fails to bounds-check structurally corrupted layer hierarchies.
	// If a graph's `h.layers` is allocated but empty, `searchPoint` initializes as `nil`,
	// causing a SIGSEGV index-out-of-bounds at offset 0x10 during `CosineDistance` calls.
	if e.graph.Len() > 0 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("vector engine: intercepted native HNSW search panic from corrupt topology. Bypassing search natively.", "component", "vector", "panic", r)
				}
			}()
			nodes = e.graph.Search(targetVec, k)
		}()
	}
	e.mu.RUnlock()

	var results []string
	var topScore float64
	for i, n := range nodes {
		results = append(results, n.Key)
		if i == 0 {
			// Convert Distance to Score (lower distance = higher score)
			topScore = 1.0 - float64(hnsw.CosineDistance(targetVec, n.Value))
		}
	}

	// 🛡️ TELEMETRY: Native increment within the mathematical kernel
	telemetry.SearchMetrics.VectorSearches.Add(1)
	if len(nodes) > 0 {
		for {
			oldBits := telemetry.SearchMetrics.TotalConfidenceScore.Load()
			oldScore := math.Float64frombits(oldBits)

			var newScore float64
			if oldScore == 0 {
				newScore = topScore
			} else {
				// EMA: 15% Latest Value + 85% Historic Ceiling
				newScore = (topScore * 0.15) + (oldScore * 0.85)
			}

			newBits := math.Float64bits(newScore)
			if telemetry.SearchMetrics.TotalConfidenceScore.CompareAndSwap(oldBits, newBits) {
				break
			}
		}
	}

	return results, nil
}

// ScoredResult pairs a URN key with its actual cosine similarity score.
type ScoredResult struct {
	Key   string
	Score float64
}

// SearchWithScores queries the HNSW index and returns actual cosine similarity scores
// for every returned node, replacing rank-based approximations with mathematically precise values.
func (e *Engine) SearchWithScores(ctx context.Context, intent string, k int) ([]ScoredResult, error) {
	if !e.VectorEnabled() {
		return nil, fmt.Errorf("cannot search: Vector capability is offline")
	}

	queryCtx := WithTaskType(ctx, "RETRIEVAL_QUERY")

	targetVec, err := e.embedder.Embed(queryCtx, intent)
	if err != nil {
		return nil, err
	}

	e.mu.RLock()
	var nodes []hnsw.Node[string]
	if e.graph.Len() > 0 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("vector engine: intercepted native HNSW SearchWithScores panic from corrupt topology.", "component", "vector", "panic", r)
				}
			}()
			nodes = e.graph.Search(targetVec, k)
		}()
	}
	e.mu.RUnlock()

	results := make([]ScoredResult, 0, len(nodes))
	for _, n := range nodes {
		cosineScore := 1.0 - float64(hnsw.CosineDistance(targetVec, n.Value))
		results = append(results, ScoredResult{Key: n.Key, Score: cosineScore})
	}

	telemetry.SearchMetrics.VectorSearches.Add(1)
	if len(results) > 0 {
		for {
			oldBits := telemetry.SearchMetrics.TotalConfidenceScore.Load()
			oldScore := math.Float64frombits(oldBits)

			var newScore float64
			if oldScore == 0 {
				newScore = results[0].Score
			} else {
				// EMA: 15% Latest Value + 85% Historic Ceiling
				newScore = (results[0].Score * 0.15) + (oldScore * 0.85)
			}

			newBits := math.Float64bits(newScore)
			if telemetry.SearchMetrics.TotalConfidenceScore.CompareAndSwap(oldBits, newBits) {
				break
			}
		}
	}

	return results, nil
}

// SearchByNode looks up a pre-embedded URN natively within the HNSW graph and computes actual Cosine Distances.
// 🛡️ PERFORMS ZERO API REQUESTS: Fast local execution securely bypassing MCP JSON-RPC timeouts.
func (e *Engine) SearchByNode(ctx context.Context, urn string, k int) ([]ScoredResult, error) {
	if !e.VectorEnabled() {
		return nil, fmt.Errorf("cannot search: Vector capability is offline")
	}

	e.mu.RLock()
	targetVec, ok := e.graph.Lookup(urn)
	if !ok {
		e.mu.RUnlock()
		return nil, fmt.Errorf("urn %s not found in vector graph", urn)
	}

	var nodes []hnsw.Node[string]
	if e.graph.Len() > 0 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("vector engine: intercepted native HNSW SearchByNode panic from corrupt topology.", "component", "vector", "panic", r)
				}
			}()
			nodes = e.graph.Search(targetVec, k+1) // Request k+1 because it will find itself
		}()
	}
	e.mu.RUnlock()

	results := make([]ScoredResult, 0, len(nodes))
	for _, n := range nodes {
		// Skip self entirely natively
		if n.Key == urn {
			continue
		}
		cosineScore := 1.0 - float64(hnsw.CosineDistance(targetVec, n.Value))
		results = append(results, ScoredResult{Key: n.Key, Score: cosineScore})
	}

	telemetry.SearchMetrics.VectorSearches.Add(1)
	if len(results) > 0 {
		for {
			oldBits := telemetry.SearchMetrics.TotalConfidenceScore.Load()
			oldScore := math.Float64frombits(oldBits)

			var newScore float64
			if oldScore == 0 {
				newScore = results[0].Score
			} else {
				// EMA: 15% Latest Value + 85% Historic Ceiling
				newScore = (results[0].Score * 0.15) + (oldScore * 0.85)
			}

			newBits := math.Float64bits(newScore)
			if telemetry.SearchMetrics.TotalConfidenceScore.CompareAndSwap(oldBits, newBits) {
				break
			}
		}
	}

	// Truncate to limit k just in case self was not found natively
	if len(results) > k {
		results = results[:k]
	}

	return results, nil
}

// contains checks if a string contains a substring (case-insensitive helper).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && len(substr) > 0 &&
			(s[0:len(substr)] == substr ||
				containsLower(s, substr)))
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			tc := substr[j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if tc >= 'A' && tc <= 'Z' {
				tc += 32
			}
			if sc != tc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
