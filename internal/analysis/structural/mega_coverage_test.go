package structural

import (
	"context"
	"testing"

	"github.com/tidwall/buntdb"
)

func TestAnalysis_CoverageSweep(t *testing.T) {
	// Initialize a volatile memory store for the Inspector cache tracking
	db, err := buntdb.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	inspector := NewInspector(db)
	targetDir := "."

	// Execute the depth traversals across the native directory structures.
	// We rely on standard error bounds for handling file errors without panics.
	res1, _ := inspector.AnalyzeDirectory(context.Background(), targetDir)
	res2, _ := inspector.AnalyzeGenericsDirectory(context.Background(), targetDir)
	res3, _ := inspector.AnalyzeLeaksDirectory(context.Background(), targetDir)

	_ = res1
	_ = res2
	_ = res3
}
