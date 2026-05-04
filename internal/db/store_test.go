package db

import (
	"testing"

	"github.com/tidwall/buntdb"
)

func TestStoreLifecycle(t *testing.T) {
	store, err := InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	sessionID := "lifecycle-test"
	
	// Test LoadSession when not exists
	_, err = store.LoadSession(sessionID)
	if err != buntdb.ErrNotFound {
		t.Errorf("Expected ErrNotFound when loading non-existent session, got %v", err)
	}

	// Test SaveSession
	session := NewSessionState(sessionID)
	err = store.SaveSession(session)
	if err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}

	// Test LoadSession when exists
	loaded, err := store.LoadSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}
	if loaded.SessionID != sessionID {
		t.Errorf("Expected session ID %q, got %q", sessionID, loaded.SessionID)
	}

	// Test UpdateCurrentStep
	err = store.UpdateCurrentStep(sessionID, "step_1")
	if err != nil {
		t.Errorf("Failed to update current step: %v", err)
	}
	loaded, _ = store.LoadSession(sessionID)
	if loaded.CurrentStep != "step_1" {
		t.Errorf("Expected CurrentStep to be 'step_1', got %q", loaded.CurrentStep)
	}

	// Test AppendStandard
	err = store.AppendStandard(sessionID, "std1")
	if err != nil {
		t.Errorf("Failed to append standard: %v", err)
	}
	loaded, _ = store.LoadSession(sessionID)
	if len(loaded.Standards) != 1 || loaded.Standards[0] != "std1" {
		t.Errorf("AppendStandard failed")
	}

	// Test AppendStepStatus
	err = store.AppendStepStatus(sessionID, "step_1", "done")
	if err != nil {
		t.Errorf("Failed to append step status: %v", err)
	}
	loaded, _ = store.LoadSession(sessionID)
	if loaded.StepStatus["step_1"] != "done" {
		t.Errorf("AppendStepStatus failed")
	}

	// Test SaveBlueprint
	bp := &Blueprint{
		ComplexityScores: map[string]int{"feature1": 5},
	}
	err = store.SaveBlueprint(sessionID, bp)
	if err != nil {
		t.Errorf("Failed to save blueprint: %v", err)
	}
	loaded, _ = store.LoadSession(sessionID)
	if loaded.Blueprint == nil || loaded.Blueprint.ComplexityScores["feature1"] != 5 {
		t.Errorf("SaveBlueprint failed")
	}

	// Test DeleteSession
	err = store.DeleteSession(sessionID)
	if err != nil {
		t.Errorf("Failed to delete session: %v", err)
	}
	_, err = store.LoadSession(sessionID)
	if err != buntdb.ErrNotFound {
		t.Errorf("Expected ErrNotFound loading deleted session, got: %v", err)
	}
}

func TestStoreUpdateSessionNonExistent(t *testing.T) {
	store, _ := InitStore()
	defer store.Close()

	err := store.updateSession("does-not-exist", func(s *SessionState) {})
	if err != buntdb.ErrNotFound {
		t.Errorf("Expected ErrNotFound updating non-existent session, got: %v", err)
	}
}
