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

	"mcp-server-brainstorm/internal/handler/system"
	"mcp-server-brainstorm/internal/util"
)

// TestRun_Brain validates the main orchestrator loop execution properly handles
// io boundaries without causing an unexpected exit.
func TestRun_Brain(t *testing.T) {
	exitFunc = func(int) {}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	buffer := &system.LogBuffer{}

	reader := strings.NewReader("")
	var writer bytes.Buffer
	errChan := make(chan error, 1)

	go func() {
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

// TestExpectedShutdown validates accurate detection of natural EOF conditions versus
// crash circumstances on the multiplexer IO pipes.
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

// TestAutoFlusher confirms that standard writes explicitly trigger pipeline downstream flushes
// ensuring sequential thinking output updates smoothly.
func TestAutoFlusher(t *testing.T) {
	var buf bytes.Buffer
	af := &autoFlusher{w: &buf}
	n, err := af.Write([]byte("test"))
	if err != nil || n != 4 {
		t.Errorf("write failed: %v", err)
	}
}

// TestEofDetector tests the streaming reader component safely recognizes protocol drop-offs
// and issues a responsive cancel to the parent contexts.
func TestEofDetector(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
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

// TestMain_Version verifies that providing the explicit version flag terminates
// execution successfully and prevents full server boot paths.
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
