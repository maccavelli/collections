package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tidwall/buntdb"
)

func TestStoreMetrics(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test_metrics.db")
	defer os.Remove(dbPath)

	store, err := InitStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	store.DBSize()
	store.View(func(tx *buntdb.Tx) error {
		time.Sleep(1 * time.Millisecond)
		return nil
	})
	store.Update(func(tx *buntdb.Tx) error {
		time.Sleep(1 * time.Millisecond)
		return nil
	})

	avgRead, avgWrite, ops := store.GetAndResetLatency()
	if avgRead == 0 || avgWrite == 0 || ops != 2 {
		t.Errorf("Expected latencies to be recorded, got %d, %d, ops=%d", avgRead, avgWrite, ops)
	}

	avgRead2, avgWrite2, ops2 := store.GetAndResetLatency()
	if avgRead2 != 0 || avgWrite2 != 0 || ops2 != 0 {
		t.Errorf("Expected latencies to be reset")
	}

	if store.DBEntries() == 0 {
		// Just ensuring it doesn't panic
	}
}

func TestSessionMetadata(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test_meta.db")
	defer os.Remove(dbPath)

	store, err := InitStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	// Initial get should return empty but valid
	meta, err := store.GetSessionMetadata("test-session")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if meta.SessionID != "test-session" {
		t.Errorf("Expected test-session, got %v", meta.SessionID)
	}

	meta.ComplexityScore = 2
	err = store.SaveSessionMetadata(meta)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	meta2, err := store.GetSessionMetadata("test-session")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if meta2.ComplexityScore != 2 {
		t.Errorf("Expected score 2, got %v", meta2.ComplexityScore)
	}
}

func TestChaosGraveyard(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test_chaos.db")
	defer os.Remove(dbPath)

	store, err := InitStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	patterns := []ChaosRejection{
		{Pattern: "test-pattern-1", Reason: "bad", Severity: "high"},
	}

	err = store.SaveChaosGraveyard("golang", patterns)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if store.ChaosGraveyardCount() != 1 {
		t.Errorf("Expected 1 chaos graveyard entry, got %d", store.ChaosGraveyardCount())
	}

	retrieved, err := store.GetChaosGraveyard("golang")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if len(retrieved) != 1 || retrieved[0].Pattern != "test-pattern-1" {
		t.Errorf("Expected test-pattern-1, got %v", retrieved)
	}

	store.PurgeChaosGraveyards()
	if store.ChaosGraveyardCount() != 0 {
		t.Errorf("Expected 0 chaos graveyard entries after purge")
	}
}

func TestListSessionsAndCount(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test_list.db")
	defer os.Remove(dbPath)

	store, err := InitStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	if store.SessionCount() != 0 {
		t.Errorf("Expected 0 sessions, got %d", store.SessionCount())
	}

	session := &SessionState{SessionID: "list-test"}
	store.SaveSession(session)

	if store.SessionCount() != 1 {
		t.Errorf("Expected 1 session, got %d", store.SessionCount())
	}

	list, err := store.ListSessions()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if len(list) != 1 || list[0].SessionID != "list-test" {
		t.Errorf("Expected list-test, got %v", list)
	}
}
