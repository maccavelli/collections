package main

// Tests for the MCP filesystem server entrypoint, covering shutdown error
// classification, auto-flushing writer behaviour, and EOF detection.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"mcp-server-filesystem/internal/handler/system"
	"mcp-server-filesystem/internal/util"
)

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

// flushableBuffer is a test helper that embeds bytes.Buffer and counts
// flush invocations to verify auto-flusher integration.
type flushableBuffer struct {
	bytes.Buffer
	flushCount int
}

// Flush increments the flush counter and returns nil, allowing tests to
// assert how many times the auto-flusher triggered a flush.
func (f *flushableBuffer) Flush() error {
	f.flushCount++
	return nil
}

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

func TestRun_FS(t *testing.T) {
	exitFunc = func(int) {}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	lb := &system.LogBuffer{}
	reader := strings.NewReader("")
	var writer bytes.Buffer
	errChan := make(chan error, 1)

	go func() {
		dir := t.TempDir()
		os.Args = []string{"cmd", dir}
		err := run(ctx, cancel, []string{dir}, lb, nil, util.NopReadCloser{Reader: reader}, util.NopWriteCloser{Writer: &writer})
		errChan <- err
		close(errChan)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-errChan:
	case <-time.After(1 * time.Second):
	}
}

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
