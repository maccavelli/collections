package cmd

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsExpectedShutdownErr(t *testing.T) {
	if isExpectedShutdownErr(nil) {
		t.Errorf("expected false for nil")
	}
	if !isExpectedShutdownErr(io.EOF) {
		t.Errorf("expected true for EOF")
	}
	if !isExpectedShutdownErr(io.ErrUnexpectedEOF) {
		t.Errorf("expected true for ErrUnexpectedEOF")
	}
	if !isExpectedShutdownErr(errors.New("broken pipe")) {
		t.Errorf("expected true for broken pipe")
	}
	if isExpectedShutdownErr(errors.New("unknown error")) {
		t.Errorf("expected false for unknown error")
	}
}

func TestEOFDetector(t *testing.T) {
	reader := bytes.NewReader([]byte("test"))
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	detector := &eofDetector{
		r:      reader,
		cancel: cancel,
	}

	buf := make([]byte, 4)
	n, err := detector.Read(buf)
	if n != 4 || err != nil {
		t.Errorf("expected to read 4 bytes")
	}
	if cancelCalled {
		t.Errorf("cancel should not be called yet")
	}

	n, err = detector.Read(buf)
	if n != 0 || err != io.EOF {
		t.Errorf("expected EOF")
	}
	if !cancelCalled {
		t.Errorf("cancel should be called on EOF")
	}
}

type dummyFlusher struct {
	io.Writer
	flushed bool
}

func (f *dummyFlusher) Flush() error {
	f.flushed = true
	return nil
}

func TestAutoFlusher(t *testing.T) {
	buf := &bytes.Buffer{}
	flusher := &dummyFlusher{Writer: buf}
	auto := &autoFlusher{w: flusher}

	n, err := auto.Write([]byte("test"))
	if n != 4 || err != nil {
		t.Errorf("expected to write 4 bytes")
	}
	if !flusher.flushed {
		t.Errorf("expected Flush to be called")
	}
}

func TestLocalhostMiddleware(t *testing.T) {
	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})
	mw := &localhostMiddleware{next: next}

	// Test authorized
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)

	if !handlerCalled {
		t.Errorf("expected handler to be called for 127.0.0.1")
	}

	// Test forbidden
	handlerCalled = false
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w = httptest.NewRecorder()
	mw.ServeHTTP(w, req)

	if handlerCalled {
		t.Errorf("expected handler to NOT be called for external IP")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden, got %d", w.Code)
	}
}
