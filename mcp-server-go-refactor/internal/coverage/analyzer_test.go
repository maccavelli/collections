package coverage

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestTool_HandleTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediate cancellation to simulate timeout

	tool := &Tool{}
	resp, _, err := tool.Handle(ctx, nil, CoverageInput{Pkg: "invalid"})
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}

	if !resp.IsError {
		t.Fatal("expected error response")
	}

	found := false
	for _, c := range resp.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if strings.Contains(tc.Text, "TIMEOUT") {
				found = true
				break
			}
		}
	}

	if !found {
		t.Error("expected timeout message in response content")
	}
}
