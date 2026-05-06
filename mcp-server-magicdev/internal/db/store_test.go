package db

import (
	"fmt"
	"testing"
	"github.com/spf13/viper"
	"github.com/tidwall/buntdb"
)

func TestStoreLifecycle(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
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
	viper.Set("server.db_path", ":memory:")
	store, err := InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	err = store.updateSession("does-not-exist", func(s *SessionState) {})
	if err != buntdb.ErrNotFound {
		t.Errorf("Expected ErrNotFound updating non-existent session, got: %v", err)
	}
}

func TestBaselineStorage(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	url := "https://test.com"
	hash := "12345"
	content := "test content"

	err = store.SetBaseline(url, content, hash)
	if err != nil {
		t.Fatalf("SetBaseline failed: %v", err)
	}

	gotHash, err := store.GetBaselineHash(url)
	if err != nil || gotHash != hash {
		t.Fatalf("Hash mismatch")
	}

	gotContent, err := store.GetBaselineContent(url)
	if err != nil || gotContent != content {
		t.Fatalf("Content mismatch")
	}

	_, err = store.GetBaselineHash("missing")
	if err == nil {
		t.Errorf("Expected error for missing hash")
	}

	_, err = store.GetBaselineContent("missing")
	if err == nil {
		t.Errorf("Expected error for missing content")
	}
}

func TestSecretStorage(t *testing.T) {
	// Initialize the vault first in a temp dir
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	
	// import vault and Init it (done outside, but we just call it)
	importVaultFunc := func() {
		// Vault is imported in store.go, so we can't easily init it without importing it here.
		// Actually, I can just call SetSecret, it will call vault.Encrypt which calls vault.Init!
	}
	importVaultFunc()

	viper.Set("server.db_path", ":memory:")
	store, err := InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	err = store.SetSecret("jira", "my-secret-token")
	if err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}

	token, err := store.GetSecret("jira")
	if err != nil || token != "my-secret-token" {
		t.Fatalf("GetSecret mismatch: %v", token)
	}

	token, err = store.GetSecret("missing")
	if err != nil || token != "" {
		t.Errorf("Expected empty token for missing secret, got err: %v, token: %v", err, token)
	}
}

func TestPurgeSessions(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	// Seed 3 sessions
	for _, id := range []string{"s1", "s2", "s3"} {
		if err := store.SaveSession(NewSessionState(id)); err != nil {
			t.Fatalf("Failed to save session %s: %v", id, err)
		}
	}

	// Seed 1 baseline
	if err := store.SetBaseline("https://test.com/std", "content", "hash123"); err != nil {
		t.Fatalf("Failed to set baseline: %v", err)
	}

	// Purge sessions
	count, err := store.PurgeSessions()
	if err != nil {
		t.Fatalf("PurgeSessions failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 purged sessions, got %d", count)
	}

	// Verify sessions are gone
	for _, id := range []string{"s1", "s2", "s3"} {
		_, err := store.LoadSession(id)
		if err == nil {
			t.Errorf("Session %s should have been purged", id)
		}
	}

	// Verify baseline survived
	hash, err := store.GetBaselineHash("https://test.com/std")
	if err != nil || hash != "hash123" {
		t.Errorf("Baseline should survive session purge, got hash=%q err=%v", hash, err)
	}

	// Verify purging empty returns 0
	count, err = store.PurgeSessions()
	if err != nil {
		t.Fatalf("PurgeSessions on empty failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 purged sessions on empty db, got %d", count)
	}
}

func TestPurgeBaselines(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	// Seed 3 baselines
	for i, url := range []string{"https://a.com", "https://b.com", "https://c.com"} {
		if err := store.SetBaseline(url, "content", fmt.Sprintf("hash%d", i)); err != nil {
			t.Fatalf("Failed to set baseline %s: %v", url, err)
		}
	}

	// Seed 1 session
	if err := store.SaveSession(NewSessionState("survive-me")); err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}

	// Purge baselines
	count, err := store.PurgeBaselines()
	if err != nil {
		t.Fatalf("PurgeBaselines failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 purged baselines, got %d", count)
	}

	// Verify baselines are gone
	for _, url := range []string{"https://a.com", "https://b.com", "https://c.com"} {
		_, err := store.GetBaselineHash(url)
		if err == nil {
			t.Errorf("Baseline %s should have been purged", url)
		}
	}

	// Verify session survived
	session, err := store.LoadSession("survive-me")
	if err != nil || session == nil {
		t.Errorf("Session should survive baseline purge, got err=%v", err)
	}
}
