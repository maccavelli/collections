// Package models defines the data structures used by the
// brainstorming MCP server for sessions, gaps, events, quality
// metrics, and architectural decision records.
package models

import "time"

// Session represents a brainstorming session state persisted
// to disk as .brainstorm.json.
type Session struct {
	ProjectRoot string            `json:"project_root"`
	ProjectName string            `json:"project_name"`
	Language    string            `json:"language"`
	Status      string            `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Gaps        []Gap             `json:"gaps"`
	History     []Event           `json:"history"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	Artifacts   map[string]string `json:"artifacts,omitempty"`
}

// GetInt securely unmarshals untyped JSON numerals from the metadata map mitigating float64 panics
func (s *Session) GetInt(key string) (int, bool) {
	if s.Metadata == nil {
		return 0, false
	}
	val, ok := s.Metadata[key]
	if !ok {
		return 0, false
	}
	switch v := val.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

// UniversalPipelineInput defines the mandatory common schema for all CSSA-aware analytical tools.
// It ensures strict correlation via SessionID, eliminates fragmentation in targeting,
// and safely routes edge-case parameters via the Flags map.
type UniversalPipelineInput struct {
	SessionID    string         `json:"session_id" jsonschema:"REQUIRED: CSSA backend correlation ID and HFSC tracking ID."`
	Target       string         `json:"target" jsonschema:"REQUIRED: Absolute path, package, or workspace URI to analyze."`
	ArtifactPath string         `json:"artifact_path,omitempty" jsonschema:"Optional OS absolute path to route the generated output payload, bypassing JSON-RPC overhead."`
	Context      string         `json:"context,omitempty" jsonschema:"Optional: Contextual text required for this specific stage."`
	Flags        map[string]any `json:"flags,omitempty" jsonschema:"Optional: Key-value map for stage-specific execution flags."`
}

// UniversalPipelineOutput defines the standard response schema for all CSSA-aware analytical tools.
// It ensures HFSC state conservation and robust standalone toggling.
type UniversalPipelineOutput struct {
	ASTHash           string         `json:"ast_hash,omitempty"`
	TelemetryDisabled bool           `json:"telemetry_disabled,omitempty"`
	Data              map[string]any `json:"data"`
}

// DiscoveryResponse provides a unified view of project gaps.
type DiscoveryResponse struct {
	Summary string `json:"summary"`
	Data    struct {
		Narrative string `json:"narrative"`
		Reasoning string `json:"reasoning,omitempty"`
		Gaps      []Gap  `json:"gaps"`
		NextStep  string `json:"next_step"`
		Standards string `json:"standards,omitempty"`
	} `json:"data"`
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

// ADR represents an Architecture Decision Record.
type ADR struct {
	Summary string `json:"summary"`
	Data    struct {
		ID                 string    `json:"id"`
		Title              string    `json:"title"`
		Date               time.Time `json:"date"`
		Status             string    `json:"status"`
		Decision           string    `json:"decision"`
		RejectedAlternates string    `json:"rejected_alternates"`
		Consequences       string    `json:"consequences"`
		Narrative          string    `json:"narrative,omitempty"`
	} `json:"data"`
}

// QualityMetric represents a score and observation for a
// single quality attribute (e.g., Scalability, Security).
type QualityMetric struct {
	Attribute   string `json:"attribute"`
	Score       int    `json:"score"`
	Observation string `json:"observation"`
}

// EvolutionResult captures a structured analysis of a proposed change.
type EvolutionResult struct {
	Summary string `json:"summary"`
	Data    struct {
		Category       string `json:"category"`
		RiskLevel      string `json:"risk_level"`
		Reasoning      string `json:"reasoning,omitempty"`
		Recommendation string `json:"recommendation"`
		Narrative      string `json:"narrative,omitempty"`
	} `json:"data"`
}

// RedTeamChallenge is a compact representation of an
// adversarial persona's challenge question.
type RedTeamChallenge struct {
	Persona  string `json:"persona"`
	Question string `json:"q"`
}

// CritiqueResponse is a consolidated assessment of a design.
type CritiqueResponse struct {
	Summary string `json:"summary"`
	Data    struct {
		Narrative  string             `json:"narrative"`
		Reasoning  string             `json:"reasoning,omitempty"`
		Challenges []string           `json:"challenges"`
		Metrics    []QualityMetric    `json:"metrics"`
		RedTeam    []RedTeamChallenge `json:"red_team"`
		Standards  string             `json:"standards,omitempty"`
	} `json:"data"`
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
	Summary string `json:"summary"`
	Data    struct {
		Narrative string         `json:"narrative"`
		Forks     []DecisionFork `json:"forks"`
	} `json:"data"`
}

// STRIDEMetrics captures threat scores for specific vectors.
type STRIDEMetrics struct {
	Spoofing             int `json:"spoofing"`
	Tampering            int `json:"tampering"`
	Repudiation          int `json:"repudiation"`
	InformationLeak      int `json:"information_leak"`
	DenialOfService      int `json:"denial_of_service"`
	ElevationOfPrivilege int `json:"elevation_of_privilege"`
}

// ThreatModelResponse captures the adversarial audit output.
type ThreatModelResponse struct {
	Summary string `json:"summary"`
	Data    struct {
		Narrative       string        `json:"narrative"`
		Metrics         STRIDEMetrics `json:"metrics"`
		Vulnerabilities []string      `json:"vulnerabilities"`
		Recommendations []string      `json:"recommendations"`
	} `json:"data"`
}

// DiagramResponse models the pure Mermaid JS output.
type DiagramResponse struct {
	Summary string `json:"summary"`
	Data    struct {
		Mermaid string `json:"mermaid"`
	} `json:"data"`
}
