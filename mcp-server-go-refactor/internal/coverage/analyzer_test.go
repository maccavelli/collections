package coverage

import (
	"context"
	"os"
	"testing"
)

func TestTraceCoverage(t *testing.T) {
	// Skip if we're already running in a Trace call to avoid infinite recursion
	if os.Getenv("GO_TEST_JSON") == "1" {
		t.Skip("Skipping recursive trace")
	}
	os.Setenv("GO_TEST_JSON", "1")
	defer os.Unsetenv("GO_TEST_JSON")

	trace, err := Trace(context.Background(), "mcp-server-go-refactor/internal/coverage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trace == nil {
		t.Fatal("expected trace data")
	}
}
