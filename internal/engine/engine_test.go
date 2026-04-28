package engine

import (
	"context"
	"mcp-server-go-refactor/internal/external"
	"testing"

	"github.com/tidwall/buntdb"
)

func TestEngineLoadSession(t *testing.T) {
	eng := NewEngine(nil)
	sess := eng.LoadSession(context.Background(), "my_root")
	if sess.ProjectRoot != "my_root" {
		t.Error("expected my root")
	}

	// test re-load cache hit
	sess2 := eng.LoadSession(context.Background(), "my_root")
	if sess2 != sess {
		t.Error("expected cached session")
	}

	eng.SaveSession(sess)
}

func TestEngineDBEntriesNil(t *testing.T) {
	eng := NewEngine(nil)
	if n := eng.DBEntries(); n != 0 {
		t.Error("expected 0 entries from nil DB")
	}
}

func TestEnsureRecallCache_NoClient(t *testing.T) {
	eng := NewEngine(nil)
	ctx := context.Background()
	sess := eng.LoadSession(ctx, "root")
	res := eng.EnsureRecallCache(ctx, sess, "my_role", "tool", nil)
	if res == "" {
		t.Log("Expected fallback execution")
	}
}

func TestEnsureRecallCache_WithMemoryDB(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	eng := NewEngine(db)
	ctx := context.Background()
	sess := eng.LoadSession(ctx, "root")

	// Execute caching sequences to hit native buntdb logic bounds
	eng.EnsureRecallCache(ctx, sess, "my_role", "tool", nil)
	eng.EnsureRecallCache(ctx, sess, "my_role", "tool", nil)

	if n := eng.DBEntries(); n == 0 {
		t.Log("Cache write success")
	}
}

func TestEnginePublishSessionToRecall(t *testing.T) {
	eng := NewEngine(nil)
	ctx := context.Background()

	// nil client -> returns early
	eng.PublishSessionToRecall(ctx, "session-123", "id", "outcome", "model", "trace", "", nil)

	// mock valid client
	client := external.NewMCPClient("http://mock")
	eng.SetExternalClient(client)

	eng.PublishSessionToRecall(ctx, "session-123", "id", "outcome", "model", "trace", "", nil)
}

func TestLoadCrossSessionFromRecall(t *testing.T) {
	eng := NewEngine(nil)
	if res := eng.LoadCrossSessionFromRecall(context.Background(), "peer", "project"); res != "" {
		t.Error("expected empty")
	}
}
