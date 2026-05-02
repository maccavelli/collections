package db

// SessionState holds the pipeline state for a given session.
type SessionState struct {
	SessionID  string            `json:"session_id"`
	TechStack  string            `json:"tech_stack"`
	Standards  []string          `json:"standards"` // dynamically ingested rules
	StepStatus map[string]string `json:"step_status"` // tracking progress
}

// NewSessionState initializes a fresh session.
func NewSessionState(sessionID string) *SessionState {
	return &SessionState{
		SessionID:  sessionID,
		Standards:  make([]string, 0),
		StepStatus: make(map[string]string),
	}
}
