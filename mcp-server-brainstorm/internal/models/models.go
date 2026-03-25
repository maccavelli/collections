// Package models defines the data structures used by the
// brainstorming MCP server for sessions, gaps, events, quality
// metrics, and architectural decision records.
package models

import "time"

// Session represents a brainstorming session state persisted
// to disk as .brainstorm.json.
type Session struct {
	ProjectRoot string    `json:"project_root"`
	ProjectName string    `json:"project_name"`
	Language    string    `json:"language"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Gaps        []Gap     `json:"gaps"`
	History     []Event   `json:"history"`
}

// DiscoveryResponse provides a unified view of project gaps
// and the recommended next step.
type DiscoveryResponse struct {
	Narrative string   `json:"narrative"`
	Reasoning string   `json:"reasoning,omitempty"` // Added for smarter Agent reasoning
	SummaryMD string   `json:"summary_md"`
	Gaps      []Gap    `json:"gaps"`
	NextStep  string   `json:"next_step"`
}

// Gap represents a missing piece of information detected by the
// engine during project analysis.
type Gap struct {
	Area        string `json:"area"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// Event tracks choices or milestones in the brainstorming
// dialogue.
type Event struct {
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
}

// ADR represents an Architecture Decision Record capturing the
// context, decision, rejected alternatives, and consequences.
type ADR struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	Date               time.Time `json:"date"`
	Status             string    `json:"status"`
	Context            string    `json:"-"`
	Decision           string    `json:"decision"`
	RejectedAlternates string    `json:"rejected_alternates"`
	Consequences       string    `json:"consequences"`
	Narrative          string    `json:"narrative,omitempty"`
	SummaryMD          string    `json:"summary_md,omitempty"`
}

// QualityMetric represents a score and observation for a
// single quality attribute (e.g., Scalability, Security).
type QualityMetric struct {
	Attribute   string `json:"attribute"`
	Score       int    `json:"score"`
	Observation string `json:"observation"`
}

// EvolutionResult captures a structured analysis of a
// proposed change, including its category, risk level,
// and actionable recommendation.
type EvolutionResult struct {
	Category       string `json:"category"`
	RiskLevel      string `json:"risk_level"`
	Reasoning      string `json:"reasoning,omitempty"` // Added for systematic analysis
	Recommendation string `json:"recommendation"`
	Narrative      string `json:"narrative,omitempty"`
	SummaryMD      string `json:"summary_md,omitempty"`
}

// RedTeamChallenge is a compact representation of an
// adversarial persona's challenge question.
type RedTeamChallenge struct {
	Persona  string `json:"persona"`
	Question string `json:"q"`
}

// CritiqueResponse is a consolidated assessment of a design
// including quality rubrics, red team challenges, and
// socratic questions.
type CritiqueResponse struct {
	Narrative  string             `json:"narrative"`
	Reasoning  string             `json:"reasoning,omitempty"` // Added for Agent transparency
	SummaryMD  string             `json:"summary_md"`
	Challenges []string           `json:"challenges"`
	Metrics    []QualityMetric    `json:"metrics"`
	RedTeam    []RedTeamChallenge `json:"red_team"`
}

// DecisionFork represents an architectural choice with options.
type DecisionFork struct {
	Component      string            `json:"component"`      // e.g., "Queue", "Storage", "Auth"
	SocraticPrompt string            `json:"socraticPrompt"` // e.g., "What is the primary requirement for event ordering?"
	Options        map[string]string `json:"options"`        // e.g., {"Strict": "FIFO", "Best-Effort": "Standard"}
	Impact         string            `json:"impact"`         // Why this choice matters
	Recommendation string            `json:"recommendation"` // A grounded suggestion for an MVP
}

// ClarificationResponse is the result of a requirement analysis.
type ClarificationResponse struct {
	Narrative string         `json:"narrative"`
	Forks     []DecisionFork `json:"forks"`
	SummaryMD string         `json:"summary_md"`
}
