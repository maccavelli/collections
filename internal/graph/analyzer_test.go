package graph

import (
	"context"
	"testing"
)

func TestCycleAnalyzer(t *testing.T) {
	cycle, err := AnalyzeCycles(context.Background(), ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycle == nil {
		t.Fatal("expected cycle data")
	}
}

func TestCallGraph(t *testing.T) {
	cg, err := AnalyzeCallGraph(context.Background(), "myFunc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cg == nil {
		t.Fatal("expected callgraph data")
	}
}
