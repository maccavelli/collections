package db

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"mcp-server-magictools/internal/telemetry"
)

// FlushMetricBucket writes the 1-minute gauge snapshot and tool history to BadgerDB.
func FlushMetricBucket(store *Store, snapshot map[string]any) {
	if store == nil || store.DB == nil {
		return
	}

	// Persist the current gauges bucket
	ts := time.Now().Truncate(time.Minute).Unix()
	key := fmt.Sprintf("telemetry:bucket:%d", ts)

	b, err := json.Marshal(snapshot)
	if err == nil {
		// Store with 30-day TTL
		store.DB.Update(func(txn *badger.Txn) error {
			e := badger.NewEntry([]byte(key), b).WithTTL(30 * 24 * time.Hour)
			return txn.SetEntry(e)
		})
	}

	// Persist per-tool history
	if toolsRaw, ok := snapshot["tools"]; ok {
		if tools, ok := toolsRaw.(map[string]*telemetry.ToolMetrics); ok {
			for urn, metrics := range tools {
				flushToolHistory(store, urn, metrics)
			}
		} else if tools, ok := toolsRaw.(map[string]telemetry.ToolMetrics); ok {
			for urn, metrics := range tools {
				flushToolHistory(store, urn, &metrics)
			}
		}
	}

	// Trigger a background non-blocking sync so ReadOnly dashboards
	// instantly pick up the flush without crashing on un-truncated logs.
	go func() {
		_ = store.DB.Sync()
	}()
}

func flushToolHistory(store *Store, urn string, metrics *telemetry.ToolMetrics) {
	if metrics == nil || store.DB == nil {
		return
	}

	ts := time.Now().Unix()
	key := fmt.Sprintf("telemetry:tool:%s:%d", urn, ts)

	avgMs := int64(0)
	if metrics.Calls > 0 {
		avgMs = metrics.TotalMs / metrics.Calls
	}

	val := map[string]any{
		"calls":  metrics.Calls,
		"avg_ms": avgMs,
		"faults": metrics.Faults,
	}
	b, err := json.Marshal(val)
	if err == nil {
		store.DB.Update(func(txn *badger.Txn) error {
			e := badger.NewEntry([]byte(key), b).WithTTL(30 * 24 * time.Hour)
			return txn.SetEntry(e)
		})
	}
}
