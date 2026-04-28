package telemetry

import "sync/atomic"

// SearchMetricsRegistry holds global atomic counters for the semantic search pipeline.
type SearchMetricsRegistry struct {
	TotalSearches        atomic.Int64
	VectorSearches       atomic.Int64 // HNSW semantic path
	LexicalSearches      atomic.Int64 // Bleve BM25 fallback path
	TotalLatencyMs       atomic.Int64
	TotalConfidenceScore atomic.Uint64 // Float64 stored as Uint64 bits for moving avg
	CacheHits            atomic.Int64  // Reserved for future vector cache layer
	CacheMisses          atomic.Int64  // Reserved for future vector cache layer
	VectorWins           atomic.Int64  // RRF: vector score dominated the fusion
	LexicalWins          atomic.Int64  // RRF: lexical score dominated the fusion
}

// SearchMetrics is the global instance of search metrics.
var SearchMetrics = &SearchMetricsRegistry{}
