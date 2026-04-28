package telemetry

import (
	"sync"
	"sync/atomic"
	"time"
)

// ToolMetrics holds real-time performance data for a specific tool URN.
type ToolMetrics struct {
	Calls      int64
	TotalMs    int64
	Faults     int64
	LastCallAt int64
}

// ToolTracker maintains thread-safe metrics for all tools.
type ToolTracker struct {
	tools sync.Map
}

// Global default tracker instance
var GlobalToolTracker = &ToolTracker{}

type internalToolMetrics struct {
	calls      atomic.Int64
	totalMs    atomic.Int64
	faults     atomic.Int64
	lastCallAt atomic.Int64
}

// Record logs the execution of a tool.
func (t *ToolTracker) Record(urn string, latencyMs int64, isError bool) {
	val, ok := t.tools.Load(urn)
	var m *internalToolMetrics

	if !ok {
		// Initialize new tool metrics struct
		m = &internalToolMetrics{}
		actual, loaded := t.tools.LoadOrStore(urn, m)
		if loaded {
			// Another goroutine beat us to it, use theirs
			m = actual.(*internalToolMetrics)
		}
	} else {
		m = val.(*internalToolMetrics)
	}

	m.calls.Add(1)
	m.totalMs.Add(latencyMs)
	m.lastCallAt.Store(time.Now().UnixNano())
	if isError {
		m.faults.Add(1)
	}
}

// GetAll returns a snapshot of all observed tools.
func (t *ToolTracker) GetAll() map[string]ToolMetrics {
	snapshot := make(map[string]ToolMetrics)
	t.tools.Range(func(key, value interface{}) bool {
		urn := key.(string)
		m := value.(*internalToolMetrics)

		snapshot[urn] = ToolMetrics{
			Calls:      m.calls.Load(),
			TotalMs:    m.totalMs.Load(),
			Faults:     m.faults.Load(),
			LastCallAt: m.lastCallAt.Load(),
		}
		return true // continue iteration
	})
	return snapshot
}
