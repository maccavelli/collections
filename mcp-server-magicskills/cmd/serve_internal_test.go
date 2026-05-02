package cmd

import (
	"context"
	"errors"
	"io"
	"testing"
)

type mockReader struct {
	readFunc func([]byte) (int, error)
}

func (m *mockReader) Read(p []byte) (int, error) {
	return m.readFunc(p)
}

func TestEOFDetector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := &mockReader{
		readFunc: func(p []byte) (int, error) {
			return 0, io.EOF
		},
	}

	detector := &eofDetector{
		r:      mr,
		cancel: cancel,
	}

	p := make([]byte, 1024)
	n, err := detector.Read(p)

	if n != 0 {
		t.Errorf("expected 0 bytes, got %d", n)
	}
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF error, got %v", err)
	}

	select {
	case <-ctx.Done():
		// OK
	default:
		t.Fatal("expected context to be cancelled on EOF")
	}
}

type mockWriter struct {
	writeFunc func([]byte) (int, error)
	flushFunc func() error
}

func (m *mockWriter) Write(p []byte) (int, error) {
	return m.writeFunc(p)
}

func (m *mockWriter) Flush() error {
	if m.flushFunc != nil {
		return m.flushFunc()
	}
	return nil
}

func TestAutoFlusher(t *testing.T) {
	flushed := false
	mw := &mockWriter{
		writeFunc: func(p []byte) (int, error) {
			return len(p), nil
		},
		flushFunc: func() error {
			flushed = true
			return nil
		},
	}

	af := &autoFlusher{w: mw}
	n, err := af.Write([]byte("test"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 4 {
		t.Errorf("expected 4 bytes, got %d", n)
	}
	if !flushed {
		t.Fatal("expected Flush to be called")
	}
}
