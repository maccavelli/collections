package engine

import (
	"os"
	"testing"
)

// TestSequentialThinkingEngine verifies the thinking engine correctly links thoughts across steps and branches.
func TestSequentialThinkingEngine(t *testing.T) {
	// Disable stdout logs during unit test
	os.Setenv("DISABLE_THOUGHT_LOGGING", "true")
	defer os.Unsetenv("DISABLE_THOUGHT_LOGGING")

	eng := NewEngine()

	// Step 1
	out, err := eng.ProcessThought(ThoughtData{
		Thought:           "Starting my analysis on testing.",
		NextThoughtNeeded: true,
		ThoughtNumber:     1,
		TotalThoughts:     3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.ThoughtHistoryLength != 1 {
		t.Errorf("expected history length 1, got %d", out.ThoughtHistoryLength)
	}
	if out.TotalThoughts != 3 {
		t.Errorf("TotalThoughts: got %d, want 3", out.TotalThoughts)
	}

	// Step 2 with Branch
	branchID := "test-branch"
	fromThought := 1
	out2, err := eng.ProcessThought(ThoughtData{
		Thought:           "This is a separate possibility based on step 1.",
		NextThoughtNeeded: false,
		ThoughtNumber:     2,
		TotalThoughts:     3,
		BranchFromThought: &fromThought,
		BranchID:          &branchID,
	})
	if err != nil {
		t.Fatalf("unexpected error on branch: %v", err)
	}

	if len(out2.Branches) != 1 || out2.Branches[0] != branchID {
		t.Errorf("expected branch '%s', got %v", branchID, out2.Branches)
	}
	if out2.ThoughtHistoryLength != 2 {
		t.Errorf("expected history length 2, got %d", out2.ThoughtHistoryLength)
	}
}
