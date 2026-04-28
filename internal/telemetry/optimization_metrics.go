package telemetry

import "sync/atomic"

// OptMetricsRegistry holds global atomic counters for the three primary
// content optimization pipelines.
type OptMetricsRegistry struct {
	SqueezeBypassCount      atomic.Int64
	SqueezeTruncations      atomic.Int64
	TotalRawBytes           atomic.Int64 // Token-Value: pre-squeeze payload size
	TotalSqueezedBytes      atomic.Int64 // Token-Value: post-squeeze payload size
	HFSCReassemblySuccesses atomic.Int64
	HFSCReassemblyFails     atomic.Int64
	HFSCSweptStale          atomic.Int64
	HFSCActiveStreams       atomic.Int64 // gauge: len(r.sessions)
	CSSAOffloadBytes        atomic.Int64
	CSSASyncOperations      atomic.Int64
}

// OptMetrics is the global instance of optimization metrics.
var OptMetrics = &OptMetricsRegistry{}
