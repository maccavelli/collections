package handler

import (
	"testing"
	"mcp-server-magicdev/internal/db"
)

func TestTruncateString(t *testing.T) {
	if truncateString("hello", 10) != "hello" {
		t.Error("expected hello")
	}
	if truncateString("hello world", 5) != "hello...[truncated]" {
		t.Error("expected truncated string")
	}
}

func TestMergeQuestions(t *testing.T) {
	a := []db.GranularQuestion{{Topic: "A", Question: "Q1"}}
	b := []db.GranularQuestion{{Topic: "A", Question: "Q1"}, {Topic: "B", Question: "Q2"}}
	merged := mergeQuestions(a, b)
	if len(merged) != 2 {
		t.Errorf("expected 2 merged questions, got %d", len(merged))
	}
}

func TestMergeStringSlices(t *testing.T) {
	a := []string{"A", "B"}
	b := []string{"B", "C"}
	merged := mergeStringSlices(a, b)
	if len(merged) != 3 {
		t.Errorf("expected 3 merged strings, got %d", len(merged))
	}
}

func TestDecisionCovers(t *testing.T) {
	decisions := []db.ArchitecturalDecision{
		{Topic: "Database", Decision: "Use Postgres"},
	}
	
	if !decisionCovers(decisions, "Database") {
		t.Error("expected true for direct topic match")
	}
	if !decisionCovers(decisions, "Postgres") {
		t.Error("expected true for decision match")
	}
	if decisionCovers(decisions, "Redis") {
		t.Error("expected false for no match")
	}
	if decisionCovers(decisions, "Do") {
		t.Error("expected false for short string")
	}
}

func TestAllFatalFlawsResolved(t *testing.T) {
	synthesis := &db.SynthesisResolution{
		Decisions: []db.ArchitecturalDecision{
			{Topic: "Security", Decision: "Use JWT"},
		},
	}
	
	chaosResolved := &db.ChaosAnalysis{
		FatalFlaws: []db.SecurityItem{
			{Category: "Security"},
		},
	}
	if !allFatalFlawsResolved(chaosResolved, synthesis) {
		t.Error("expected true when flaw is covered")
	}
	
	chaosUnresolved := &db.ChaosAnalysis{
		FatalFlaws: []db.SecurityItem{
			{Category: "Performance"},
		},
	}
	if allFatalFlawsResolved(chaosUnresolved, synthesis) {
		t.Error("expected false when flaw is not covered")
	}
	
	chaosEmpty := &db.ChaosAnalysis{}
	if !allFatalFlawsResolved(chaosEmpty, synthesis) {
		t.Error("expected true when no flaws exist")
	}
}
