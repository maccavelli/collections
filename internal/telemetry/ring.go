// Package telemetry provides functionality for the telemetry subsystem.
package telemetry

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"
)

// LogEntry defines the LogEntry structure.
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
}

// RingBuffer is a fixed-size circular buffer for telemetry.
type RingBuffer struct {
	mu       sync.Mutex
	capacity int
	entries  []LogEntry
	head     int
	count    int
}

// NewRingBuffer creates a new RingBuffer of the specified capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		capacity: capacity,
		entries:  make([]LogEntry, capacity),
	}
}

// Write implements io.Writer. It uses a non-blocking TryLock strategy
// so that a deep Go panic dump won't deadlock if the buffer is locked elsewhere.
func (r *RingBuffer) Write(p []byte) (n int, err error) {
	if !r.mu.TryLock() {
		// If we can't acquire the lock instantly, drop the log.
		// Survival of os.Stderr stream is more important than telemetry during a crash.
		return len(p), nil
	}
	defer r.mu.Unlock()

	lines := strings.SplitSeq(strings.TrimSpace(string(p)), "\n")
	for line := range lines {
		if line == "" {
			continue
		}
		r.entries[r.head] = LogEntry{
			Timestamp: time.Now(),
			Level:     "SYSTEM", // generic log intercept
			Message:   line,
		}
		r.head = (r.head + 1) % r.capacity
		if r.count < r.capacity {
			r.count++
		}
	}
	return len(p), nil
}

// AddLog adds a structured log entry to the buffer safely.
func (r *RingBuffer) AddLog(level, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries[r.head] = LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}
	r.head = (r.head + 1) % r.capacity
	if r.count < r.capacity {
		r.count++
	}
}

// String returns the buffer contents formatted as text.
func (r *RingBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var buf bytes.Buffer
	start := 0
	if r.count == r.capacity {
		start = r.head
	}

	for i := 0; i < r.count; i++ {
		idx := (start + i) % r.capacity
		e := r.entries[idx]
		buf.WriteString(fmt.Sprintf("[%s] [%s] %s\n", e.Timestamp.Format(time.RFC3339), e.Level, e.Message))
	}
	return buf.String()
}
