package engine

import (
	"context"
	"testing"

	"mcp-server-brainstorm/internal/models"
)

func TestResolveSafePath_Adopt(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	thesis := models.ThesisDocument{}
	thesis.Data.Pillars = []models.DialecticPillar{
		{Name: "Type Safety & Generics", Score: 8, Finding: "Strong signal"},
	}
	counter := models.CounterThesisReport{
		Pillars: []models.DialecticPillar{
			{Name: "Type Safety & Generics", Score: 8, Finding: "Safe"},
		},
	}

	resolutions := eng.ResolveSafePath(context.Background(), thesis, counter)
	found := false
	for _, r := range resolutions {
		if r.Pillar == "Type Safety & Generics" {
			found = true
			if r.Resolution != "ADOPT" {
				t.Errorf("expected ADOPT for high/high, got %q", r.Resolution)
			}
			if r.Contradiction {
				t.Error("expected no contradiction for ADOPT")
			}
		}
	}
	if !found {
		t.Error("Type Safety & Generics pillar not found in resolutions")
	}
}

func TestResolveSafePath_Mitigation(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	thesis := models.ThesisDocument{}
	thesis.Data.Pillars = []models.DialecticPillar{
		{Name: "Modernization", Score: 8, Finding: "Strong signal"},
	}
	counter := models.CounterThesisReport{
		Pillars: []models.DialecticPillar{
			{Name: "Modernization", Score: 5, Finding: "Moderate risk"},
		},
	}

	resolutions := eng.ResolveSafePath(context.Background(), thesis, counter)
	for _, r := range resolutions {
		if r.Pillar == "Modernization" {
			if r.Resolution != "ADOPT_WITH_MITIGATION" {
				t.Errorf("expected ADOPT_WITH_MITIGATION for high/mid, got %q", r.Resolution)
			}
		}
	}
}

func TestResolveSafePath_Aporia(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	thesis := models.ThesisDocument{}
	thesis.Data.Pillars = []models.DialecticPillar{
		{Name: "Modularization", Score: 9, Finding: "Strong signal"},
	}
	counter := models.CounterThesisReport{
		Pillars: []models.DialecticPillar{
			{Name: "Modularization", Score: 2, Finding: "Dangerous"},
		},
	}

	resolutions := eng.ResolveSafePath(context.Background(), thesis, counter)
	for _, r := range resolutions {
		if r.Pillar == "Modularization" {
			if r.Resolution != "APORIA" {
				t.Errorf("expected APORIA for high/low, got %q", r.Resolution)
			}
			if !r.Contradiction {
				t.Error("expected contradiction for APORIA")
			}
		}
	}
}

func TestResolveSafePath_Skip(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	thesis := models.ThesisDocument{}
	thesis.Data.Pillars = []models.DialecticPillar{
		{Name: "Efficiency", Score: 3, Finding: "Low signal"},
	}
	counter := models.CounterThesisReport{
		Pillars: []models.DialecticPillar{
			{Name: "Efficiency", Score: 8, Finding: "Safe"},
		},
	}

	resolutions := eng.ResolveSafePath(context.Background(), thesis, counter)
	for _, r := range resolutions {
		if r.Pillar == "Efficiency" {
			if r.Resolution != "SKIP" {
				t.Errorf("expected SKIP for low thesis, got %q", r.Resolution)
			}
		}
	}
}

func TestResolveSafePath_Review(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	thesis := models.ThesisDocument{}
	thesis.Data.Pillars = []models.DialecticPillar{
		{Name: "Reliability", Score: 5, Finding: "Moderate"},
	}
	counter := models.CounterThesisReport{
		Pillars: []models.DialecticPillar{
			{Name: "Reliability", Score: 5, Finding: "Moderate"},
		},
	}

	resolutions := eng.ResolveSafePath(context.Background(), thesis, counter)
	for _, r := range resolutions {
		if r.Pillar == "Reliability" {
			if r.Resolution != "REVIEW" {
				t.Errorf("expected REVIEW for mid/mid, got %q", r.Resolution)
			}
		}
	}
}

func TestResolveSafePath_MixedPillars(t *testing.T) {
	eng := NewEngine("/tmp", nil)
	thesis := models.ThesisDocument{}
	thesis.Data.Pillars = []models.DialecticPillar{
		{Name: "Type Safety & Generics", Score: 8},
		{Name: "Modernization", Score: 6},
		{Name: "Modularization", Score: 9},
		{Name: "Efficiency", Score: 4},
		{Name: "Reliability", Score: 7},
		{Name: "Maintainability", Score: 5},
	}
	counter := models.CounterThesisReport{
		Pillars: []models.DialecticPillar{
			{Name: "Type Safety & Generics", Score: 7},
			{Name: "Modernization", Score: 7},
			{Name: "Modularization", Score: 5},
			{Name: "Efficiency", Score: 8},
			{Name: "Reliability", Score: 7},
			{Name: "Maintainability", Score: 2},
		},
	}

	resolutions := eng.ResolveSafePath(context.Background(), thesis, counter)
	if len(resolutions) != 6 {
		t.Fatalf("expected 6 resolutions, got %d", len(resolutions))
	}

	expected := map[string]string{
		"Type Safety & Generics": "ADOPT",
		"Modernization":          "SKIP", // 6 thesis, 7 skeptic
		"Modularization":         "ADOPT_WITH_MITIGATION",
		"Efficiency":             "SKIP", // 4 thesis, 8 skeptic
		"Reliability":            "ADOPT",
		"Maintainability":        "SKIP", // 5 thesis (4-6), 2 skeptic (<4) → default SKIP
	}

	for _, r := range resolutions {
		if exp, ok := expected[r.Pillar]; ok {
			if r.Resolution != exp {
				t.Errorf("pillar %q: expected %q, got %q", r.Pillar, exp, r.Resolution)
			}
		}
	}
}

func TestComputeSafePathVerdict(t *testing.T) {
	tests := []struct {
		name     string
		input    []models.AporiaResolution
		expected string
	}{
		{"empty", nil, "REVIEW"},
		{"all adopt", []models.AporiaResolution{
			{Resolution: "ADOPT"},
			{Resolution: "ADOPT_WITH_MITIGATION"},
		}, "APPROVE"},
		{"has aporia", []models.AporiaResolution{
			{Resolution: "ADOPT"},
			{Resolution: "APORIA", Contradiction: false},
		}, "REVIEW"},
		{"has contradiction", []models.AporiaResolution{
			{Resolution: "APORIA", Contradiction: true},
		}, "REJECT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict := ComputeSafePathVerdict(tt.input)
			if verdict != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, verdict)
			}
		})
	}
}
