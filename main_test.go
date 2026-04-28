package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
)

// TestIsExpectedShutdownErr verifies that the function correctly identifies expected networking errors.
func TestIsExpectedShutdownErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"EOF", io.EOF, true},
		{"unexpected EOF", io.ErrUnexpectedEOF, true},
		{"broken pipe", fmt.Errorf("broken pipe"), true},
		{"connection reset", fmt.Errorf("connection reset by peer"), true},
		{"use of closed", fmt.Errorf("use of closed network connection"), true},
		{"bad file descriptor", fmt.Errorf("bad file descriptor"), true},
		{"client is closing", fmt.Errorf("client is closing"), true},
		{"connection closed", fmt.Errorf("connection closed"), true},
		{"file already closed", fmt.Errorf("file already closed"), true},
		{"random error", fmt.Errorf("random error"), false},
		{"wrapped EOF", fmt.Errorf("wrapped: %w", io.EOF), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isExpectedShutdownErr(tc.err)
			if got != tc.want {
				t.Errorf("isExpectedShutdownErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestAutoFlusher_Write verifies that the autoFlusher correctly buffers output data.
func TestAutoFlusher_Write(t *testing.T) {
	var buf bytes.Buffer
	af := &autoFlusher{w: &buf}
	n, err := af.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Errorf("Write: n=%d, err=%v", n, err)
	}
	if buf.String() != "hello" {
		t.Errorf("got %q, want %q", buf.String(), "hello")
	}
}

// TestAutoFlusher_WithFlusher verifies the auto-flusher flushes on newline-terminated writes.
func TestAutoFlusher_WithFlusher(t *testing.T) {
	fb := &flushableBuffer{}
	af := &autoFlusher{w: fb}

	// Non-newline write should NOT trigger flush
	af.Write([]byte("data"))
	if fb.flushCount != 0 {
		t.Errorf("expected 0 flushes for non-newline write, got %d", fb.flushCount)
	}

	// Newline-terminated write should trigger flush
	af.Write([]byte("data\n"))
	if fb.flushCount != 1 {
		t.Errorf("expected 1 flush for newline write, got %d", fb.flushCount)
	}
}

type flushableBuffer struct {
	bytes.Buffer
	flushCount int
}

// Flush implements the flusher interface for mock testing.
func (f *flushableBuffer) Flush() error {
	f.flushCount++
	return nil
}

// TestEOFDetector_TriggersCancel verifies that an EOF triggers the cancellation context.
func TestEOFDetector_TriggersCancel(t *testing.T) {
	cancelled := false
	r := &eofDetector{
		r:      bytes.NewReader(nil), // empty reader returns EOF
		cancel: func() { cancelled = true },
	}
	buf := make([]byte, 1)
	_, err := r.Read(buf)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
	if !cancelled {
		t.Error("expected cancel to be called on EOF")
	}
}

// TestEOFDetector_NormalRead verifies that normal reads do not trigger the cancellation context.
func TestEOFDetector_NormalRead(t *testing.T) {
	cancelled := false
	r := &eofDetector{
		r:      bytes.NewReader([]byte("hello")),
		cancel: func() { cancelled = true },
	}
	buf := make([]byte, 5)
	n, err := r.Read(buf)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes, got %d", n)
	}
	if cancelled {
		t.Error("cancel should not be called for normal reads")
	}
}

// TestMain_Version verifies that the -version flag correctly outputs the version and exits.
func TestMain_Version(t *testing.T) {
	os.Args = []string{"cmd", "-version"}
	exited := false
	exitFunc = func(_ int) {
		exited = true
	}
	main()
	if !exited {
		t.Error("main should have exited")
	}
}
