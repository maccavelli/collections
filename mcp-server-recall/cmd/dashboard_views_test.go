package cmd

import (
	"testing"
)

func TestDashboardViews(t *testing.T) {
	snapshot := map[string]any{
		"runtime": map[string]any{
			"memory_mb":  100,
			"goroutines": 10,
			"uptime_sec": 60,
			"num_gc":     2,
			"cpu_usage":  5.5,
		},
		"storage": map[string]any{
			"lsm_bytes":  1000,
			"vlog_bytes": 2000,
		},
		"memory_gc": map[string]any{
			"sweeps":       1,
			"pruned_nodes": 2,
		},
		"bleve": map[string]any{
			"documents": 10,
			"queues":    0,
			"drift":     0,
		},
		"analytics": map[string]any{
			"avg_rpc_latency_ms": 1.2,
			"cache_hits":         10,
			"cache_misses":       2,
			"db_hits":            5,
			"db_misses":          1,
			"rpc_payload_bytes":  100,
		},
		"ast": map[string]any{
			"disable_drift": false,
			"exclude_dirs":  0,
			"parsed_files":  100,
		},
		"taxonomy": map[string]any{
			"memories":  10,
			"sessions":  5,
			"standards": 2,
			"projects":  1,
		},
		"network": map[string]any{
			"active_sessions": 2,
			"transport":       "stdio",
		},
		"security": map[string]any{
			"boundary_violations": 0,
			"auth_failures":       0,
		},
		"config": map[string]any{
			"version":        "1.0",
			"db_path":        "/tmp",
			"log_level":      "info",
			"env_gomemlimit": "",
		},
	}

	logs := []TelemetryLog{
		{Time: "2026-05-09T10:00:00Z", Level: "INFO", Msg: "msg", Pkg: "pkg"},
		{Time: "2026-05-09T10:00:00Z", Level: "DEBUG", Msg: "msg", Tool: "tool"},
		{Time: "2026-05-09T10:00:00Z", Level: "WARN", Msg: "msg", Pkg: "pkg"},
		{Time: "2026-05-09T10:00:00Z", Level: "ERROR", Msg: "msg", Pkg: "pkg"},
	}

	uiState := NewUIState()

	for i := 1; i <= 11; i++ {
		uiState.SetActiveTab(int32(i))
		res := renderPtermDashboard(snapshot, logs, uiState)
		if res == "" {
			t.Errorf("Expected non-empty string for tab %d", i)
		}
	}
}
