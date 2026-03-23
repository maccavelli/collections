package layout

import (
	"context"
	"testing"
)

type TestStruct struct {
	A bool
	B int64
	C bool
}

func TestAnalyzeStructAlignment(t *testing.T) {
	result, err := AnalyzeStructAlignment(context.Background(), "TestStruct", "mcp-server-go-refactor/internal/layout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected alignment data")
	}
	if result.CurrentSizeBytes != 24 {
		t.Errorf("expected current size 24, got %d", result.CurrentSizeBytes)
	}
	if result.OptimalSizeBytes != 16 {
		t.Errorf("expected optimal size 16, got %d", result.OptimalSizeBytes)
	}
}
