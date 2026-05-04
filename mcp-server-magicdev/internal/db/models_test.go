package db

import (
	"testing"
)

func TestNewSessionState(t *testing.T) {
	sessionID := "test-session-123"
	session := NewSessionState(sessionID)

	if session.SessionID != sessionID {
		t.Errorf("Expected SessionID %q, got %q", sessionID, session.SessionID)
	}

	if session.Standards != nil {
		t.Error("Standards slice should be nil for omitzero")
	}

	if session.StepStatus == nil {
		t.Error("StepStatus map should not be nil")
	}

	if session.AporiaResolutions != nil {
		t.Error("AporiaResolutions slice should be nil for omitzero")
	}

	if len(session.StepStatus) != 0 {
		t.Error("Collections should be initialized as empty")
	}
}
