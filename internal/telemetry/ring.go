// Package telemetry provides functionality for the telemetry subsystem.
package telemetry

import "sync"

// RingBuffer is a thread-safe, fixed-size circular buffer for telemetry data.
// It bypasses the need for costly OS-level file locking and disk I/O, allowing
// the Bubbletea dashboard loop to render metrics from memory natively.
type RingBuffer struct {
	mu     sync.RWMutex
	buffer []string
	size   int
	head   int // index to insert next item
	count  int // total items in buffer
}

// GlobalRing is the global telemetry ring buffer instance, sized to 5,000 events.
// The ring buffer serves only in-process consumers (e.g. the get_internal_logs MCP tool).
// Cross-process consumers (e.g. the dashboard) read telemetry from BuntDB session state.
var GlobalRing = NewRingBuffer(5000)

// NewRingBuffer allocates a circular array of the defined size.
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		buffer: make([]string, size),
		size:   size,
	}
}

// Push appends a telemetry string onto the ring in O(1) time.
func (r *RingBuffer) Push(item string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buffer[r.head] = item
	r.head = (r.head + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

// Snapshot returns a copy of the buffer contents in chronological order.
// This executes in nanoseconds, eliminating file descriptor polling bottlenecks.
func (r *RingBuffer) Snapshot() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.count == 0 {
		return []string{}
	}

	out := make([]string, r.count)

	// If the buffer isn't completely full, just copy from index 0 to head
	if r.count < r.size {
		copy(out, r.buffer[:r.head])
		return out
	}

	// If the buffer is full (wrapped), start reading from head to end, then 0 to head
	firstPart := r.size - r.head
	copy(out[:firstPart], r.buffer[r.head:])
	copy(out[firstPart:], r.buffer[:r.head])
	return out
}
