package memory

import (
	"context"
	"fmt"
	"os"
	"testing"
)

func TestMemoryStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "test-key"
	content := "test content"
	tags := []string{"tag1", "tag2"}

	// Test Save
	err = store.Save(ctx, key, content, tags)
	if err != nil {
		t.Errorf("Save failed: %v", err)
	}

	// Test Get
	rec, err := store.Get(ctx, key)
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if rec.Content != content {
		t.Errorf("expected content %s, got %s", content, rec.Content)
	}
	if len(rec.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(rec.Tags))
	}

	// Test Search
	results, err := store.Search(ctx, "content", "", 0)
	if err != nil {
		t.Errorf("Search failed: %v", err)
	}
	if _, ok := results[key]; !ok {
		t.Errorf("Search did not find key")
	}

	// Test GetRecent
	recent, err := store.GetRecent(ctx, 1)
	if err != nil {
		t.Errorf("GetRecent failed: %v", err)
	}
	if len(recent) != 1 {
		t.Errorf("expected 1 recent record, got %d", len(recent))
	}

	// Test GetStats
	count, size, err := store.GetStats()
	if err != nil {
		t.Errorf("GetStats failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
	if size < 0 {
		t.Errorf("expected size >= 0, got %d", size)
	}

	// Test Delete
	err = store.Delete(ctx, key)
	if err != nil {
		t.Errorf("Delete failed: %v", err)
	}
	_, err = store.Get(ctx, key)
	if err == nil {
		t.Errorf("expected error for deleted key, got nil")
	}

	// Test Clear
	_ = store.Save(ctx, "k2", "c2", nil)
	err = store.Clear(ctx)
	if err != nil {
		t.Errorf("Clear failed: %v", err)
	}
	results, _ = store.ListKeys(ctx)
	if len(results) != 0 {
		t.Errorf("expected 0 records after clear, got %d", len(results))
	}
}

func BenchmarkMemoryStore(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "recall-bench-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewMemoryStore(tmpDir)
	defer store.Close()

	ctx := context.Background()
	// Seed with 1000 items
	for i := 0; i < 1000; i++ {
		_ = store.Save(ctx, fmt.Sprintf("key-%d", i), fmt.Sprintf("content block for record %d", i), []string{"tag"})
	}

	b.Run("GetRecent-10", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = store.GetRecent(ctx, 10)
		}
	})

	b.Run("Search-FullScan", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = store.Search(ctx, "record 500", "", 0)
		}
	})
}
