package astutil

import (
	"context"
	"slices"
	"testing"
)

type MyInterface interface {
	DoWork()
}

type MyStruct struct{}

func (s *MyStruct) DoWork() {}

func TestAnalyzeInterface(t *testing.T) {
	analysis, err := AnalyzeInterface(context.Background(), "mcp-server-go-refactor/internal/astutil", "MyStruct", "MyInterface")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if analysis == nil {
		t.Fatal("expected analysis data")
	}
	if !analysis.IsCompatible {
		t.Errorf("expected compatible, got missing: %v", analysis.MissingMethods)
	}
}
func TestExtractInterface(t *testing.T) {
	result, err := ExtractInterface(context.Background(), "mcp-server-go-refactor/internal/astutil", "MyStruct")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result data")
	}
	found := slices.Contains(result.Methods, "DoWork()")
	if !found {
		t.Errorf("expected to find DoWork() in methods, got: %v", result.Methods)
	}
}
