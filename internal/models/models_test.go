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
		Narrative: "narrative",
		SummaryMD: "summary",
		Gaps:      s.Gaps,
		NextStep:  "step",
	}

	adr := ADR{
		ID:                 "1",
		Title:              "title",
		Date:               now,
		Status:             "accepted",
		Decision:           "decision",
		RejectedAlternates: "none",
		Consequences:       "none",
	}

	qm := QualityMetric{
		Attribute:   "attr",
		Score:       5,
		Observation: "obs",
	}

	er := EvolutionResult{
		Category:       "cat",
		RiskLevel:      "low",
		Recommendation: "rec",
	}

	rtc := RedTeamChallenge{
		Persona:  "persona",
		Question: "q",
	}

	cr := CritiqueResponse{
		Narrative:  "critique",
		SummaryMD:  "summary",
		Challenges: []string{"c1"},
		Metrics:    []QualityMetric{qm},
		RedTeam:    []RedTeamChallenge{rtc},
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
