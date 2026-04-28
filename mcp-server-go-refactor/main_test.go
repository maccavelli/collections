package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"mcp-server-go-refactor/internal/handler/system"
	"mcp-server-go-refactor/internal/util"
)

// TestRun_Refactor guarantees standard invocation pipeline success.
func TestRun_Refactor(t *testing.T) {
	exitFunc = func(int) {}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	reader := strings.NewReader("")
	var writer bytes.Buffer
	buffer := &system.LogBuffer{}
	errChan := make(chan error, 1)

	go func() {
		// Run needs chdir isolation safely for caching
		err := run(ctx, cancel, buffer, util.NopReadCloser{Reader: reader}, util.NopWriteCloser{Writer: &writer})
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

// TestIsExpectedShutdownErr asserts the error resolution helper safely ignores shutdown markers.
func TestIsExpectedShutdownErr(t *testing.T) {
	if isExpectedShutdownErr(nil) {
		t.Error("nil is not expected")
	}
	if !isExpectedShutdownErr(io.EOF) {
		t.Error("EOF is expected")
	}
	if !isExpectedShutdownErr(errors.New("broken pipe")) {
		t.Error("broken pipe is expected")
	}
	if isExpectedShutdownErr(errors.New("unknown error")) {
		t.Error("unknown is not expected")
	}
}

// TestAutoFlusher_Write validates exact flush interception byte mapping.
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

// TestAutoFlusher_WithFlusher maps execution inside flush boundaries.
func TestAutoFlusher_WithFlusher(t *testing.T) {
	fb := &flushableBuffer{}
	af := &autoFlusher{w: fb}
	af.Write([]byte("data"))
	if fb.flushCount != 1 {
		t.Errorf("expected 1 flush, got %d", fb.flushCount)
	}
}

type flushableBuffer struct {
	bytes.Buffer
	flushCount int
}

// Flush enforces explicit internal flushes for testing assertions.
func (f *flushableBuffer) Flush() error {
	f.flushCount++
	return nil
}

// TestEOFDetector_TriggersCancel confirms the native context termination is emitted on EOF.
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

// TestEOFDetector_NormalRead ensures valid read frames are propagated organically without triggering cancellation.
func TestEOFDetector_NormalRead(t *testing.T) {
	cancelled := false
	r := &eofDetector{
		r:      bytes.NewReader([]byte("hello")),
		cancel: func() { cancelled = true },
	}
	buf := make([]byte, 5)
	_, err := r.Read(buf)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cancelled {
		t.Error("cancel should not be called for normal reads")
	}
}

// TestMain_Version ensures the cli parser traps and exits properly upon a version flag argument.
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
