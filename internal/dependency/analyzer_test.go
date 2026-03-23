package dependency

import (
	"context"
	"testing"
)

func TestAnalyzeImpact(t *testing.T) {
	// Not testing an actual external network request in unit tests usually, 
	// but we'll try something that's already in our module like mcp-go
	impact, err := Analyze(context.Background(), "github.com/mark3labs/mcp-go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if impact == nil {
		t.Fatal("expected impact data")
	}
}
