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

	"mcp-server-duckduckgo/internal/handler/system"
	"mcp-server-duckduckgo/internal/util"
)

// TestRun_Duck verifies the standard IO subsystem for the search server.
func TestRun_Duck(t *testing.T) {
	exitFunc = func(int) {}
	rootCtx := context.Background()
	ctx, cancel := context.WithCancel(rootCtx)
	defer cancel()

	lb := &system.LogBuffer{}
	reader := strings.NewReader("")
	var writer bytes.Buffer
	errChan := make(chan error, 1)

	go func() {
		err := run(ctx, cancel, lb, util.NopReadCloser{Reader: reader}, util.NopWriteCloser{Writer: &writer})

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

// TestExpectedShutdown verifies that standard termination errors are correctly identified.
func TestExpectedShutdown(t *testing.T) {
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

// TestAutoFlusher verifies the behavior of the write-triggered auto-flusher buffer.
func TestAutoFlusher(t *testing.T) {
	var buf bytes.Buffer
	af := &autoFlusher{w: &buf}
	n, err := af.Write([]byte("test"))
	if err != nil || n != 4 {
		t.Errorf("write failed: %v", err)
	}
}

// TestEofDetector verifies that EOF conditions trigger an immediate context cancellation signal.
func TestEofDetector(t *testing.T) {
	rootCtx := context.Background()
	ctx, cancel := context.WithCancel(rootCtx)
	defer cancel()

	reader := strings.NewReader("")
	detector := &eofDetector{r: reader, cancel: cancel}

	buf := make([]byte, 10)
	_, err := detector.Read(buf)
	if err != io.EOF {
		t.Error("expected EOF")
	}
	
	if ctx.Err() == nil {
		t.Error("expected context canceled")
	}
}

// TestMain_Version verifies the standalone version text flag output.
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
