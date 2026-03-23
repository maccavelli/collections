package pruner

import (
	"context"
	"testing"
)

func TestPruneDeadCode(t *testing.T) {
	result, err := PruneDeadCode(context.Background(), "mcp-server-go-refactor/internal/pruner")
	if err != nil {
		t.Fatalf("failed to prune dead code: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}
}
