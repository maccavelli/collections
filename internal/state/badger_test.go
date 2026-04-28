package state

import (
	"os"
	"testing"
)

func TestNewStore(t *testing.T) {
	dbPath := t.TempDir()
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("Database directory was not created")
	}
}

func TestStore_Efficacy(t *testing.T) {
	dbPath := t.TempDir()
	store, _ := NewStore(dbPath)
	defer store.Close()

	skill := "test-skill"

	// Test Record Success
	err := store.RecordEfficacy("", skill, true)
	if err != nil {
		t.Fatalf("RecordEfficacy failed: %v", err)
	}

	stats, err := store.GetEfficacy("", skill)
	if err != nil {
		t.Fatalf("GetEfficacy failed: %v", err)
	}

	if stats.Successes != 1 {
		t.Errorf("expected 1 success, got %d", stats.Successes)
	}

	// Test Record Failure
	_ = store.RecordEfficacy("", skill, false)
	stats, _ = store.GetEfficacy("", skill)
	if stats.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", stats.Failures)
	}

	hits, misses := store.GetMetrics()
	if hits == 0 {
		t.Error("expected hits > 0")
	}
	_ = misses
}

func TestStore_CountEntries(t *testing.T) {
	dbPath := t.TempDir()
	store, _ := NewStore(dbPath)
	defer store.Close()

	count := store.CountEntries()
	if count != 0 {
		t.Errorf("expected 0 entries, got %d", count)
	}

	_ = store.RecordEfficacy("", "skill1", true)
	_ = store.RecordEfficacy("", "skill2", true)

	count = store.CountEntries()
	if count != 2 {
		t.Errorf("expected 2 entries, got %d", count)
	}
}

func TestStore_Close(t *testing.T) {
	dbPath := t.TempDir()
	store, _ := NewStore(dbPath)

	err := store.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify context is cancelled
	select {
	case <-store.ctx.Done():
		// OK
	default:
		t.Fatal("expected store context to be cancelled on Close")
	}
}
