package engine

import (
	"context"
	"testing"
)

func TestCounterThesis_TypeSafety(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	report, err := eng.GenerateCounterThesis(context.Background(), "proposes deep type parameter nesting with interface wrappers and proxy adapter layers", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Pillars) != 6 {
		t.Fatalf("expected 6 pillars, got %d", len(report.Pillars))
	}
	pillar := report.Pillars[0]
	if pillar.Name != "Type Safety & Generics" {
		t.Errorf("expected 'Type Safety & Generics', got %q", pillar.Name)
	}
	if pillar.Score >= 7 {
		t.Errorf("expected type safety risk score < 7 for deep nesting + wrappers, got %d", pillar.Score)
	}
}

func TestCounterThesis_Modernization(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	report, err := eng.GenerateCounterThesis(context.Background(), "refactor for ergonomic and cleaner idiomatic patterns, modernize all the code", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pillar := report.Pillars[1]
	if pillar.Name != "Modernization" {
		t.Errorf("expected 'Modernization', got %q", pillar.Name)
	}
	if pillar.Score >= 7 {
		t.Errorf("expected modernization risk score < 7 for speculative refactor, got %d", pillar.Score)
	}
}

func TestCounterThesis_Modularization(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	report, err := eng.GenerateCounterThesis(context.Background(), "split the handler and extract the processing into a new package with decomposition", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pillar := report.Pillars[2]
	if pillar.Name != "Modularization" {
		t.Errorf("expected 'Modularization', got %q", pillar.Name)
	}
	if pillar.Score >= 7 {
		t.Errorf("expected modularization risk score < 7 for split/extract, got %d", pillar.Score)
	}
}

func TestCounterThesis_Efficiency(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	report, err := eng.GenerateCounterThesis(context.Background(), "uses reflect for config parsing and interface{} boxing, append without preallocated cap", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pillar := report.Pillars[3]
	if pillar.Name != "Efficiency" {
		t.Errorf("expected 'Efficiency', got %q", pillar.Name)
	}
	if pillar.Score >= 7 {
		t.Errorf("expected efficiency risk score < 7 for reflect/interface{}/append, got %d", pillar.Score)
	}
}

func TestCounterThesis_Reliability(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	report, err := eng.GenerateCounterThesis(context.Background(), "adds sync.Mutex for state protection and chan communication channel patterns", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pillar := report.Pillars[4]
	if pillar.Name != "Reliability" {
		t.Errorf("expected 'Reliability', got %q", pillar.Name)
	}
	if pillar.Score >= 7 {
		t.Errorf("expected reliability risk score < 7 for sync/channel, got %d", pillar.Score)
	}
}

func TestCounterThesis_Maintainability(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	report, err := eng.GenerateCounterThesis(context.Background(), "changes exported public api signature, adds dependency in go.mod, serialization json: tag changes", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pillar := report.Pillars[5]
	if pillar.Name != "Maintainability" {
		t.Errorf("expected 'Maintainability', got %q", pillar.Name)
	}
	if pillar.Score >= 7 {
		t.Errorf("expected maintainability risk score < 7 for API/dep/tag changes, got %d", pillar.Score)
	}
}

func TestCounterThesis_Clean(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	report, err := eng.GenerateCounterThesis(context.Background(), "simple utility function that processes data", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Verdict != "APPROVE" {
		t.Errorf("expected APPROVE for clean input, got %q", report.Verdict)
	}
}

func TestCounterThesis_VerdictLogic(t *testing.T) {
	eng := NewEngine("/tmp", nil)

	tests := []struct {
		name            string
		input           string
		expectedVerdict string
	}{
		{
			name:            "clean code approves",
			input:           "straightforward data processing utility",
			expectedVerdict: "APPROVE",
		},
		{
			name:            "heavy risk rejects",
			input:           "deep type nesting with interface wrapper proxy adapter, refactor for cleaner modernized ergonomic patterns, split and extract into new package decomposition, reflect boxing interface{} usage append, mutex sync. channel chan locking, export public api signature dependency go.mod json: tag serialization changes",
			expectedVerdict: "REJECT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := eng.GenerateCounterThesis(context.Background(), tt.input, "", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if report.Verdict != tt.expectedVerdict {
				t.Errorf("expected verdict %q, got %q", tt.expectedVerdict, report.Verdict)
			}
		})
	}
}
