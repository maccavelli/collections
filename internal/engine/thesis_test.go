package engine

import (
	"context"
	"testing"
)

func TestGenerateThesis_TypeSafety(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	doc, err := eng.GenerateThesis(context.Background(), "uses interface{} and type assertions for conversion, reflect for config parsing", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.Data.Pillars) != 6 {
		t.Fatalf("expected 6 pillars, got %d", len(doc.Data.Pillars))
	}
	pillar := doc.Data.Pillars[0]
	if pillar.Name != "Type Safety & Generics" {
		t.Errorf("expected 'Type Safety & Generics', got %q", pillar.Name)
	}
	if pillar.Score <= 5 {
		t.Errorf("expected type safety score > 5 for interface{}/reflect input, got %d", pillar.Score)
	}
}

func TestGenerateThesis_Modernization(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	doc, err := eng.GenerateThesis(context.Background(), "struct tags use omitempty, the code creates temporary variables with var tmp = val then return &tmp", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pillar := doc.Data.Pillars[1]
	if pillar.Name != "Modernization" {
		t.Errorf("expected 'Modernization', got %q", pillar.Name)
	}
	if pillar.Score <= 5 {
		t.Errorf("expected modernization score > 5 for omitempty input, got %d", pillar.Score)
	}
}

func TestGenerateThesis_Modularization(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	doc, err := eng.GenerateThesis(context.Background(), "the handler mixes http endpoints with database queries and crypto auth in one file", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pillar := doc.Data.Pillars[2]
	if pillar.Name != "Modularization" {
		t.Errorf("expected 'Modularization', got %q", pillar.Name)
	}
	if pillar.Score <= 5 {
		t.Errorf("expected modularization score > 5 for mixed concerns, got %d", pillar.Score)
	}
}

func TestGenerateThesis_Efficiency(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	doc, err := eng.GenerateThesis(context.Background(), "uses reflect for config parsing, calls append in loops without make capacity", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pillar := doc.Data.Pillars[3]
	if pillar.Name != "Efficiency" {
		t.Errorf("expected 'Efficiency', got %q", pillar.Name)
	}
	if pillar.Score <= 5 {
		t.Errorf("expected efficiency score > 5 for reflect/append input, got %d", pillar.Score)
	}
}

func TestGenerateThesis_Reliability(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	doc, err := eng.GenerateThesis(context.Background(), "processes data from remote api with error handling, no retry or fallback", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pillar := doc.Data.Pillars[4]
	if pillar.Name != "Reliability" {
		t.Errorf("expected 'Reliability', got %q", pillar.Name)
	}
	if pillar.Score <= 5 {
		t.Errorf("expected reliability score > 5 for missing context, got %d", pillar.Score)
	}
}

func TestGenerateThesis_Maintainability(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	doc, err := eng.GenerateThesis(context.Background(), "legacy code with TODO markers and deprecated functions, no go doc comments", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pillar := doc.Data.Pillars[5]
	if pillar.Name != "Maintainability" {
		t.Errorf("expected 'Maintainability', got %q", pillar.Name)
	}
	if pillar.Score <= 5 {
		t.Errorf("expected maintainability score > 5 for todo/deprecated input, got %d", pillar.Score)
	}
}

func TestGenerateThesis_Empty(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	doc, err := eng.GenerateThesis(context.Background(), "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.Data.Pillars) != 6 {
		t.Fatalf("expected 6 pillars, got %d", len(doc.Data.Pillars))
	}
	for _, p := range doc.Data.Pillars {
		if p.Score < 1 || p.Score > 10 {
			t.Errorf("pillar %q has out-of-range score %d", p.Name, p.Score)
		}
	}
	if doc.Verdict != "APPROVE" && doc.Verdict != "REVIEW" && doc.Verdict != "REJECT" {
		t.Errorf("unexpected verdict %q", doc.Verdict)
	}
}

func TestGenerateThesis_WithTraceMap(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	traceMap := map[string]interface{}{
		"interfaces":      []interface{}{"Foo", "Bar", "Baz", "Qux", "Quux", "Corge"},
		"dead_code":       "3 unreachable functions detected",
		"coverage":        30.5,
		"total_functions": 25.0,
	}
	doc, err := eng.GenerateThesis(context.Background(), "standard go project code", "", traceMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.Data.Pillars) != 6 {
		t.Fatalf("expected 6 pillars, got %d", len(doc.Data.Pillars))
	}
	// Interface count > 5 should boost type safety
	tsScore := doc.Data.Pillars[0].Score
	if tsScore <= 5 {
		t.Errorf("expected type safety score > 5 with 6 interfaces, got %d", tsScore)
	}
}
