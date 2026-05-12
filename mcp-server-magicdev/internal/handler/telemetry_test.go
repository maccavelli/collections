package handler

import (
	"testing"
	"time"

	"mcp-server-magicdev/internal/db"
)

func TestRecordStepTiming(t *testing.T) {
	session := &db.SessionState{
		SessionID: "123",
	}

	// First step — RecordStepTiming is called at step completion
	RecordStepTiming(session, "step1")
	session.CurrentStep = "step1"

	if len(session.StepTimings) != 1 {
		t.Errorf("Expected 1 timing, got %v", len(session.StepTimings))
	}
	// Step is now completed immediately since RecordStepTiming is called at the END of handler execution
	if session.StepTimings["step1"].CompletedAt == "" {
		t.Errorf("Expected step1 completed immediately")
	}
	if session.StepTimings["step1"].DurationMs < 0 {
		t.Errorf("Expected non-negative duration, got %d", session.StepTimings["step1"].DurationMs)
	}

	time.Sleep(10 * time.Millisecond)

	// Second step
	RecordStepTiming(session, "step2")
	session.CurrentStep = "step2"

	if len(session.StepTimings) != 2 {
		t.Errorf("Expected 2 timings, got %v", len(session.StepTimings))
	}
	if session.StepTimings["step1"].CompletedAt == "" {
		t.Errorf("Expected step1 completed")
	}
	if session.StepTimings["step2"].CompletedAt == "" {
		t.Errorf("Expected step2 completed immediately")
	}
}

func TestLogSessionHash(t *testing.T) {
	session := &db.SessionState{
		SessionID: "123",
	}
	// Should just log and not panic
	LogSessionHash(session, "test-step")
}

func TestCheckPayloadCompleteness(t *testing.T) {
	session := db.NewSessionState("test-session")

	// Should store ratio on session
	CheckPayloadCompleteness(session, "test-step", 5, 10)
	if ratio, ok := session.StepHydrations["test-step"]; !ok || ratio != 0.5 {
		t.Errorf("Expected hydration ratio 0.5, got %v (exists: %v)", ratio, ok)
	}

	// Full hydration
	CheckPayloadCompleteness(session, "full-step", 10, 10)
	if ratio := session.StepHydrations["full-step"]; ratio != 1.0 {
		t.Errorf("Expected hydration ratio 1.0, got %v", ratio)
	}

	// Zero total — should not panic or store
	CheckPayloadCompleteness(session, "zero-step", 0, 0)
	if _, ok := session.StepHydrations["zero-step"]; ok {
		t.Error("Expected no hydration entry for zero total")
	}

	// Nil session — should not panic
	CheckPayloadCompleteness(nil, "nil-step", 3, 5)
}
