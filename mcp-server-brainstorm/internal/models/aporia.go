// Package models provides functionality for the models subsystem.
package models

// AporiaReport represents the highly compressed dialectic critique model.
// Utilizes 'omitzero' (or 'omitempty' in JSON mapping) so that absent flaws
// drop from the payload cleanly, fulfilling the 'Squeezed Matrix' CSSA constraint.
type AporiaReport struct {
	RefusalToProceed   bool               `json:"refusal_to_proceed,omitempty"`
	ZeroValueTrap      string             `json:"zero_value_trap,omitempty"`
	GenericBloat       string             `json:"generic_bloat,omitempty"`
	GreenTeaLocality   string             `json:"green_tea_locality,omitempty"`
	RedTeamSuggestions []string           `json:"red_team_suggestions,omitempty"`
	RedTeamScore       int                `json:"red_team_score,omitempty"`
	RedTeamVerdict     string             `json:"red_team_verdict,omitempty"`
	Resolutions        []AporiaResolution `json:"resolutions,omitempty"`
	SafePathVerdict    string             `json:"safe_path_verdict,omitempty"` // APPROVE, REVIEW, REJECT
}

// AporiaResolution captures the cross-reference result for a single
// pillar pair between thesis and antithesis.
type AporiaResolution struct {
	Pillar         string `json:"pillar"` // Shared pillar name
	ThesisScore    int    `json:"thesis_score"`
	SkepticScore   int    `json:"skeptic_score"`
	Contradiction  bool   `json:"contradiction"`
	Resolution     string `json:"resolution"` // ADOPT, ADOPT_WITH_MITIGATION, APORIA, SKIP, REVIEW
	SafePathAction string `json:"safe_path_action"`
}

// CounterThesisReport is the structured output of the antithesis_skeptic tool.
// It challenges thesis proposals against 6 generalized performance/robustness
// dimensions using the same DialecticPillar type for 1:1 alignment.
type CounterThesisReport struct {
	Summary      string            `json:"summary"`
	Verdict      string            `json:"verdict"` // APPROVE, REVIEW, REJECT
	Pillars      []DialecticPillar `json:"pillars"` // Same 6 pillar names as thesis
	AporiaReport                   // Embedded
}
