package telemetry

import "time"

type MetricPayload struct {
	// System Metrics
	UptimeSeconds    int64  `json:"uptime_seconds"`
	MemoryAllocBytes uint64 `json:"memory_alloc_bytes"`
	ActiveGoroutines int    `json:"active_goroutines"`
	GCPauseNs        uint64 `json:"gc_pause_ns"`

	// Session Metrics
	NetworkBytesRead    int64  `json:"network_bytes_read"`
	NetworkBytesWritten int64  `json:"network_bytes_written"`
	PipelineStage       string `json:"pipeline_stage"`
	TrifectaReviewCount int    `json:"trifecta_review_count"`
	SessionContextBytes int    `json:"session_context_bytes"`
	SessionTokensEst    int    `json:"session_tokens_est"`
}

var (
	// TelemetryPorts are the UDP ports used for dashboard telemetry (serve listens, dashboard connects).
	TelemetryPorts = []int{49151, 49152, 49153, 49154, 49155}
	// EmissionInterval controls how frequently the serve process pushes metrics to the dashboard.
	// 500ms provides near-real-time updates without excessive ReadMemStats overhead.
	EmissionInterval = 500 * time.Millisecond
)
