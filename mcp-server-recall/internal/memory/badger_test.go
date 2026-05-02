package memory

import (
	"maps"
	"mcp-server-recall/internal/config"

	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"mcp-server-recall/internal/search"
)

func TestMemoryStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "test-key"
	content := "test content"
	tags := []string{"tag1", "tag2"}

	t.Run("Save_And_Get", func(t *testing.T) {
		_, err := store.Save(ctx, "", key, content, "test-cat", tags, "", 0)
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		rec, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if rec.Content != content {
			t.Errorf("expected content %s, got %s", content, rec.Content)
		}
		if len(rec.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(rec.Tags))
		}
	})

	t.Run("EdgeCases", func(t *testing.T) {
		_, err := store.Save(ctx, "", "empty-key", "", "cat", nil, "", 0)
		if err != nil {
			t.Fatalf("Save empty content failed: %v", err)
		}

		_, err = store.Get(ctx, "non-existent-key-999")
		if err == nil {
			t.Errorf("expected error getting non existent key")
		}
	})

	t.Run("Search_Fuzzy", func(t *testing.T) {
		results, err := store.Search(ctx, "content", "", 0)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		found := false
		for _, r := range results {
			if r.Key == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Search did not find key")
		}
	})

	t.Run("GetRecent", func(t *testing.T) {
		recent, err := store.GetRecent(ctx, 1)
		if err != nil {
			t.Fatalf("GetRecent failed: %v", err)
		}
		if len(recent) != 1 {
			t.Errorf("expected 1 recent record, got %d", len(recent))
		}
	})

	t.Run("GetStats", func(t *testing.T) {
		count, size, err := store.GetStats()
		if err != nil {
			t.Fatalf("GetStats failed: %v", err)
		}
		if count != 2 {
			t.Errorf("expected count 2, got %d", count)
		}
		if size < 0 {
			t.Errorf("expected size >= 0, got %d", size)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := store.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		_, err = store.Get(ctx, key)
		if err == nil {
			t.Errorf("expected error for deleted key, got nil")
		}
	})

	t.Run("DedupAtWrite", func(t *testing.T) {
		// Save original memory
		_, _ = store.Save(ctx, "", "near-dup-1", "This is a detailed memory about Go performance optimization.", "go-perf", []string{"go", "perf"}, "", 0)

		// Save similar content with dedup enabled (threshold 0.3 = very aggressive)
		result, err := store.Save(ctx, "", "near-dup-2", "Go performance optimization: brief note about speed.", "go-perf", []string{"go"}, "", 0.3)
		if err != nil {
			t.Fatalf("Save with dedup failed: %v", err)
		}
		if result.Action != "merged" {
			t.Errorf("expected action 'merged', got %q", result.Action)
		}

		// Save distinct content — should create new record
		result, err = store.Save(ctx, "", "distinct", "Something completely different about Python.", "other", []string{"python"}, "", 0.3)
		if err != nil {
			t.Fatalf("Save distinct failed: %v", err)
		}
		if result.Action != "created" {
			t.Errorf("expected action 'created', got %q", result.Action)
		}

		// Verify merged record has unioned tags
		rec1, err := store.Get(ctx, "near-dup-1")
		if err != nil {
			t.Fatalf("Primary key lost: %v", err)
		}
		if len(rec1.Tags) < 2 {
			t.Errorf("expected >= 2 merged tags, got %d", len(rec1.Tags))
		}
	})

	t.Run("ListCategories", func(t *testing.T) {
		cats, err := store.ListCategories(ctx)
		if err != nil {
			t.Fatalf("ListCategories failed: %v", err)
		}
		if cats["go-perf"] == 0 {
			t.Errorf("expected go-perf category to have multiple records")
		}
		if cats["other"] != 1 {
			t.Errorf("expected other category to have 1 record")
		}
	})

	t.Run("ListKeys_And_SearchLimit", func(t *testing.T) {
		keys, err := store.ListKeys(ctx)
		if err != nil {
			t.Fatalf("ListKeys failed: %v", err)
		}
		if len(keys) == 0 {
			t.Errorf("expected to find keys")
		}

		results, err := store.Search(ctx, "go optimization", "", 1)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result due to limit")
		}
	})

	t.Run("Clear", func(t *testing.T) {
		_, _ = store.Save(ctx, "", "k2", "c2", "tmp", nil, "", 0)
		err := store.Clear(ctx)
		if err != nil {
			t.Fatalf("Clear failed: %v", err)
		}
		resultsKeys, _ := store.ListKeys(ctx)
		if len(resultsKeys) != 0 {
			t.Errorf("expected 0 records after clear, got %d", len(resultsKeys))
		}
	})
}

func BenchmarkMemoryStore(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "recall-bench-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	defer store.Close()

	ctx := context.Background()
	// Seed with 1000 items
	for i := range 1000 {
		_, _ = store.Save(ctx, "", fmt.Sprintf("key-%d", i), fmt.Sprintf("content block for record %d", i), "perf-test", []string{"tag"}, "", 0)
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

// TestDatabaseSizeConstraints ensures the DB stays small under write load.
// This is a regression test for the vlog bloat bug where 5-6 entries caused
// the database to grow to 2GB+.
func TestDatabaseSizeConstraints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-size-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}

	ctx := context.Background()

	// Write 50 entries of ~10KB each
	largeContent := strings.Repeat("x", 10*1024)
	for i := range 50 {
		key := fmt.Sprintf("entry-%d", i)
		_, err := store.Save(ctx, "", key, largeContent, "test", []string{"size-test"}, "", 0)
		if err != nil {
			t.Fatalf("Save failed on entry %d: %v", i, err)
		}
	}

	// Overwrite each entry 10 times (simulates real-world update patterns)
	for round := range 10 {
		for i := range 50 {
			key := fmt.Sprintf("entry-%d", i)
			content := fmt.Sprintf("round-%d: %s", round, largeContent[:5*1024])
			_, err := store.Save(ctx, "", key, content, "test", []string{"size-test", "updated"}, "", 0)
			if err != nil {
				t.Fatalf("Update failed on round %d, entry %d: %v", round, i, err)
			}
		}
	}

	store.Close()

	// Walk the DB directory and sum all file sizes
	var totalBytes int64
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read DB dir: %v", err)
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		totalBytes += info.Size()
	}

	const maxSizeMB = 50
	totalMB := totalBytes >> 20
	t.Logf("Total DB size after 550 writes: %d MB", totalMB)

	if totalMB > maxSizeMB {
		t.Errorf("DB size %d MB exceeds %d MB limit — vlog bloat regression", totalMB, maxSizeMB)
	}
}

func TestSaveBatch_Success(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-batch-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	entries := []BatchEntry{
		{Key: "batch-1", Value: "value-1", Category: "test", Tags: []string{"a"}},
		{Key: "batch-2", Value: "value-2", Category: "test", Tags: []string{"b"}},
		{Key: "batch-3", Value: "value-3"},
	}

	stored, batchErrors, err := store.SaveBatch(ctx, entries)
	if err != nil {
		t.Fatalf("SaveBatch failed: %v", err)
	}
	if stored != 3 {
		t.Errorf("expected 3 stored, got %d", stored)
	}
	if len(batchErrors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(batchErrors))
	}

	// Verify all entries are retrievable via Get.
	for _, e := range entries {
		rec, err := store.Get(ctx, e.Key)
		if err != nil {
			t.Errorf("Get(%q) failed: %v", e.Key, err)
			continue
		}
		if rec.Content != e.Value {
			t.Errorf("Get(%q): content = %q, want %q", e.Key, rec.Content, e.Value)
		}
	}
}

func TestSaveBatch_Empty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-batch-empty-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	stored, _, err := store.SaveBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("SaveBatch(nil) should succeed, got: %v", err)
	}
	if stored != 0 {
		t.Errorf("expected 0 stored, got %d", stored)
	}
}

func TestSaveBatch_OverLimit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-batch-limit-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	entries := make([]BatchEntry, 101)
	for i := range entries {
		entries[i] = BatchEntry{Key: fmt.Sprintf("k%d", i), Value: "v"}
	}

	_, _, err = store.SaveBatch(context.Background(), entries)
	if err == nil {
		t.Fatal("expected error for batch exceeding 100 entries, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetBatch_MixedResults(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-getbatch-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Save two entries individually.
	if _, err := store.Save(ctx, "", "exists-1", "v1", "cat", nil, "", 0); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := store.Save(ctx, "", "exists-2", "v2", "cat", nil, "", 0); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	found, missing, err := store.GetBatch(ctx, []string{"exists-1", "exists-2", "nope-1", "nope-2"})
	if err != nil {
		t.Fatalf("GetBatch failed: %v", err)
	}

	if len(found) != 2 {
		t.Errorf("expected 2 found, got %d", len(found))
	}
	if len(missing) != 2 {
		t.Errorf("expected 2 missing, got %d", len(missing))
	}

	if _, ok := found["exists-1"]; !ok {
		t.Error("expected exists-1 in found")
	}
	if _, ok := found["exists-2"]; !ok {
		t.Error("expected exists-2 in found")
	}
}

func TestGetBatch_OverLimit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-getbatch-limit-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewMemoryStore(context.Background(), tmpDir, "", 0, config.New("test").BatchSettings())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	keys := make([]string, 101)
	for i := range keys {
		keys[i] = fmt.Sprintf("k%d", i)
	}

	_, _, err = store.GetBatch(context.Background(), keys)
	if err == nil {
		t.Fatal("expected error for batch exceeding 100 keys, got nil")
	}
}

type mockSearchEngine struct {
	indexed map[string]*search.Document
}

func (m *mockSearchEngine) Rebuild(ctx context.Context, docs map[string]*search.Document) error {
	maps.Copy(m.indexed, docs)
	return nil
}
func (m *mockSearchEngine) Index(id string, doc *search.Document) error {
	m.indexed[id] = doc
	return nil
}
func (m *mockSearchEngine) IndexBatch(docs map[string]*search.Document) error { return nil }
func (m *mockSearchEngine) Delete(id string) error {
	delete(m.indexed, id)
	return nil
}
func (m *mockSearchEngine) DeleteBatch(ids []string) error { return nil }
func (m *mockSearchEngine) Search(ctx context.Context, query string, keys []string, limit int) ([]search.SearchHit, error) {
	hits := []search.SearchHit{}
	if _, ok := m.indexed[query]; ok {
		hits = append(hits, search.SearchHit{ID: query, Score: 1.0})
	}
	return hits, nil
}
func (m *mockSearchEngine) SearchScoped(ctx context.Context, query string, categories []string, requiredTags []string, limit int) ([]search.SearchHit, error) {
	hits := []search.SearchHit{}
	if _, ok := m.indexed[query]; ok {
		hits = append(hits, search.SearchHit{ID: query, Score: 1.0})
	}
	return hits, nil
}
func (m *mockSearchEngine) DocCount() (uint64, error) { return uint64(len(m.indexed)), nil }
func (m *mockSearchEngine) Has(id string) (bool, error) {
	_, ok := m.indexed[id]
	return ok, nil
}
func (m *mockSearchEngine) Close() error { return nil }

func TestDriftHealing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-heal-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewMemoryStore(context.Background(), tmpDir, "", 5, config.New("test").BatchSettings())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	mockEngine := &mockSearchEngine{indexed: make(map[string]*search.Document)}
	err = store.SetSearchEngine(ctx, mockEngine)
	if err != nil {
		t.Fatalf("failed to set search engine: %v", err)
	}

	// Add keys
	store.Save(ctx, "", "key1", "val1", "cat1", nil, "", 0)
	store.Save(ctx, "", "key2", "val2", "cat2", nil, "", 0)
	store.Save(ctx, "", "key3", "val3", "cat3", nil, "", 0)

	// Simulate drift: Delete key1 directly from Mock
	delete(mockEngine.indexed, "key1")

	// Trigger Audit
	store.performAudit()

	// Verify drift was healed
	if _, ok := mockEngine.indexed["key1"]; !ok {
		t.Errorf("expected key1 to be healed and re-indexed in search engine")
	}
	if store.DriftAlerts() == 0 {
		t.Errorf("expected drift alerts to increment")
	}
}
