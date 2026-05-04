package logging

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// GlobalBuffer is the process-wide log buffer.
var GlobalBuffer = &LogBuffer{}

const (
	LogBufferLimit = 2 * 1024 * 1024 // 2MB
	LogTrimTarget  = 1 * 1024 * 1024 // 1MB
)

var secretRegex = regexp.MustCompile(`(?i)(token_|sk_|key_|secret_)[a-zA-Z0-9_-]+`)

// LogBuffer stores recent server logs in memory.
type LogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends data to the buffer, redacting secrets and trimming if over limit.
func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	redacted := secretRegex.ReplaceAll(p, []byte("[REDACTED]"))
	_, err = lb.buf.Write(redacted)
	if err != nil {
		return len(p), err
	}
	n = len(p)

	if lb.buf.Len() > LogBufferLimit {
		data := lb.buf.Bytes()
		trimPoint := len(data) - LogTrimTarget
		if idx := bytes.IndexByte(data[trimPoint:], '\n'); idx >= 0 {
			trimPoint += idx + 1
		}
		newData := make([]byte, len(data)-trimPoint)
		copy(newData, data[trimPoint:])
		lb.buf.Reset()
		if _, err := lb.buf.Write(newData); err != nil {
			return 0, fmt.Errorf("trim buffer: %w", err)
		}
	}

	return n, nil
}

// String returns the current buffer contents.
func (lb *LogBuffer) String() string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.buf.String()
}

// TailLines returns the last n lines of the buffer string.
func TailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
