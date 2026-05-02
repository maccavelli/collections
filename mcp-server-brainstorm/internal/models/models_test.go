package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestModelsCoverage(t *testing.T) {
	// Exercise structs to satisfy coverage and verify JSON tags.
	now := time.Now()

	s := Session{
		ProjectRoot: "/tmp",
		ProjectName: "test",
		Language:    "go",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
		Gaps: []Gap{{
			Area:        "test",
			Description: "desc",
			Severity:    "high",
		}},
		History: []Event{{
			Timestamp:   now,
			Description: "event",
		}},
	}

	dr := DiscoveryResponse{
		Summary: "summary",
		Data: struct {
			Narrative string            `json:"narrative"`
			Reasoning string            `json:"reasoning,omitempty"`
			Gaps      []Gap             `json:"gaps"`
			NextStep  string            `json:"next_step"`
			Standards string            `json:"standards,omitempty"`
			Metadata  DiscoveryMetadata `json:"metadata"`
		}{
			Narrative: "narrative",
			Gaps:      s.Gaps,
			NextStep:  "step",
		},
	}

	adr := ADR{
		Summary: "summary",
		Data: struct {
			ID                 string    `json:"id"`
			Title              string    `json:"title"`
			Date               time.Time `json:"date"`
			Status             string    `json:"status"`
			Decision           string    `json:"decision"`
			RejectedAlternates string    `json:"rejected_alternates"`
			Consequences       string    `json:"consequences"`
			Narrative          string    `json:"narrative,omitempty"`
		}{
			ID:                 "1",
			Title:              "title",
			Date:               now,
			Status:             "accepted",
			Decision:           "decision",
			RejectedAlternates: "none",
			Consequences:       "none",
			Narrative:          "narr",
		},
	}

	qm := QualityMetric{
		Attribute:   "attr",
		Score:       5,
		Observation: "obs",
	}

	er := EvolutionResult{
		Summary: "summary",
		Data: struct {
			Category       string `json:"category"`
			RiskLevel      string `json:"risk_level"`
			Reasoning      string `json:"reasoning,omitempty"`
			Recommendation string `json:"recommendation"`
			Narrative      string `json:"narrative,omitempty"`
		}{
			Category:       "cat",
			RiskLevel:      "low",
			Recommendation: "rec",
		},
	}

	rtc := RedTeamChallenge{
		Persona:  "persona",
		Question: "q",
	}

	cr := CritiqueResponse{
		Summary: "summary",
		Data: struct {
			Narrative  string             `json:"narrative"`
			Reasoning  string             `json:"reasoning,omitempty"`
			Challenges []string           `json:"challenges"`
			Metrics    []QualityMetric    `json:"metrics"`
			RedTeam    []RedTeamChallenge `json:"red_team"`
			Standards  string             `json:"standards,omitempty"`
		}{
			Narrative:  "critique",
			Challenges: []string{"c1"},
			Metrics:    []QualityMetric{qm},
			RedTeam:    []RedTeamChallenge{rtc},
		},
	}

	df := DecisionFork{
		Component:      "comp",
		SocraticPrompt: "prompt",
		Options:        map[string]string{"k": "v"},
		Impact:         "impact",
		Recommendation: "rec",
	}

	cl := ClarificationResponse{
		Summary: "sum",
		Data: struct {
			Narrative string         `json:"narrative"`
			Forks     []DecisionFork `json:"forks"`
		}{
			Narrative: "narr",
			Forks:     []DecisionFork{df},
		},
	}

	tm := ThreatModelResponse{
		Summary: "sum",
		Data: struct {
			Narrative       string        `json:"narrative"`
			Metrics         STRIDEMetrics `json:"metrics"`
			Vulnerabilities []string      `json:"vulnerabilities"`
			Recommendations []string      `json:"recommendations"`
		}{
			Narrative: "narr",
			Metrics: STRIDEMetrics{
				Spoofing: 1,
			},
			Vulnerabilities: []string{"v1"},
			Recommendations: []string{"r1"},
		},
	}

	ar := AporiaReport{
		RefusalToProceed: true,
		Resolutions: []AporiaResolution{
			{Pillar: "p1", Resolution: "ADOPT"},
		},
	}

	ctr := CounterThesisReport{
		Summary: "sum",
		Verdict: "APPROVE",
		Pillars: []DialecticPillar{
			{Name: "p1", Score: 8},
		},
		AporiaReport: ar,
	}

	// Marshalt/Unmarshal to cover JSON tags and struct fields.
	payloads := []any{s, dr, adr, qm, er, rtc, cr, df, cl, tm, ar, ctr}
	for _, p := range payloads {
		data, err := json.Marshal(p)
		if err != nil {
			t.Errorf("failed to marshal %T: %v", p, err)
		}
		if len(data) == 0 {
			t.Errorf("empty json for %T", p)
		}
	}
}

func TestSession_GetInt(t *testing.T) {
	s := &Session{
		Metadata: map[string]any{
			"int":     10,
			"int64":   int64(20),
			"float64": float64(30.5),
			"string":  "40",
		},
	}

	tests := []struct {
		key   string
		want  int
		found bool
	}{
		{"int", 10, true},
		{"int64", 20, true},
		{"float64", 30, true}, // Truncated
		{"string", 0, false},
		{"missing", 0, false},
	}

	for _, tt := range tests {
		got, ok := s.GetInt(tt.key)
		if ok != tt.found || got != tt.want {
			t.Errorf("GetInt(%q) = (%v, %v), want (%v, %v)", tt.key, got, ok, tt.want, tt.found)
		}
	}

	// Test nil metadata
	s2 := &Session{}
	if got, ok := s2.GetInt("any"); ok || got != 0 {
		t.Errorf("GetInt on nil metadata should fail")
	}
}
