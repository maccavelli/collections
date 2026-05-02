package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
)

func TestMultiHandler(t *testing.T) {
	h1 := slog.NewJSONHandler(io.Discard, nil)
	h2 := slog.NewTextHandler(io.Discard, nil)
	multi := NewMultiHandler(h1, h2)

	ctx := context.Background()
	if !multi.Enabled(ctx, slog.LevelInfo) {
		t.Error("expected multi to be enabled for info")
	}

	err := multi.Handle(ctx, slog.Record{Level: slog.LevelInfo, Message: "test"})
	if err != nil {
		t.Errorf("Handle failed: %v", err)
	}

	_ = multi.WithAttrs([]slog.Attr{slog.String("k", "v")})
	_ = multi.WithGroup("g")
}

func TestMcpLogHandler(t *testing.T) {
	level := &slog.LevelVar{}
	handler := NewMcpLogHandler(level)

	ctx := context.Background()
	if !handler.Enabled(ctx, slog.LevelInfo) {
		t.Error("expected enabled")
	}

	// Handle without session should return nil (graceful)
	err := handler.Handle(ctx, slog.Record{Level: slog.LevelInfo, Message: "test"})
	if err != nil {
		t.Errorf("Handle failed: %v", err)
	}

	_ = handler.WithAttrs(nil)
	_ = handler.WithGroup("")
}

func TestWireTap(t *testing.T) {
	// Just hit the lines
	wtr := &WireTapReader{Rc: io.NopCloser(io.LimitReader(nil, 0)), Ctx: context.Background()}
	_, _ = wtr.Read(make([]byte, 10))
	wtr.Close()

	wtw := &WireTapWriter{Wc: os.Stderr, Ctx: context.Background()}
	_, _ = wtw.Write([]byte("test"))
	// Skip Close to avoid closing stderr
}
