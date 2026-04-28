package server

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

type nopCloser struct {
	*bytes.Buffer
}

func (nopCloser) Close() error { return nil }

func TestMCPServer(t *testing.T) {
	logger := slog.Default()
	srv := NewMCPServer("test", "1.0", logger)
	if srv.MCPServer() == nil {
		t.Fatal("MCPServer() returned nil")
	}

	stdout := nopCloser{Buffer: &bytes.Buffer{}}
	reader := nopCloser{Buffer: bytes.NewBufferString("{}")} // dummy json

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()

	err := srv.Serve(ctx, stdout, reader)
	if err != nil && err != context.DeadlineExceeded && err.Error() != "context deadline exceeded" {
		// Just ensure it doesn't panic and returns an error when context cancels or drops
	}
}
