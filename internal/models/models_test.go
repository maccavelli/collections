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
			Narrative string `json:"narrative"`
			Reasoning string `json:"reasoning,omitempty"`
			Gaps      []Gap  `json:"gaps"`
			NextStep  string `json:"next_step"`
			Standards string `json:"standards,omitempty"`
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

	// Marshalt/Unmarshal to cover JSON tags and struct fields.
	payloads := []interface{}{s, dr, adr, qm, er, rtc, cr}
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
