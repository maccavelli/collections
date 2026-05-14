package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestIsExpectedShutdownErr(t *testing.T) {
	if isExpectedShutdownErr(nil) {
		t.Error("expected false for nil")
	}
	if !isExpectedShutdownErr(io.EOF) {
		t.Error("expected true for EOF")
	}
	if !isExpectedShutdownErr(io.ErrUnexpectedEOF) {
		t.Error("expected true for ErrUnexpectedEOF")
	}
	if !isExpectedShutdownErr(errors.New("something broken pipe here")) {
		t.Error("expected true for broken pipe")
	}
	if isExpectedShutdownErr(errors.New("unknown error")) {
		t.Error("expected false for unknown error")
	}
}

type dummyFlusher struct {
	bytes.Buffer
	flushed bool
}

func (d *dummyFlusher) Flush() error {
	d.flushed = true
	return nil
}

func TestAutoFlusher(t *testing.T) {
	df := &dummyFlusher{}
	af := &autoFlusher{w: df}

	_, err := af.Write([]byte("hello"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if df.flushed {
		t.Error("expected not flushed")
	}

	_, err = af.Write([]byte("world\n"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !df.flushed {
		t.Error("expected flushed")
	}

	df.flushed = false
	af.Close()
	if !df.flushed {
		t.Error("expected flushed on close")
	}
}

type dummyReader struct {
	err error
}

func (d *dummyReader) Read(p []byte) (n int, err error) {
	return 0, d.err
}

func TestEofDetector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ed := &eofDetector{
		r:      &dummyReader{err: io.EOF},
		cancel: cancel,
	}

	_, err := ed.Read([]byte("test"))
	if err != io.EOF {
		t.Errorf("expected EOF, got: %v", err)
	}

	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}

	err = ed.Close()
	if err != nil {
		t.Errorf("expected nil from close, got %v", err)
	}
}
