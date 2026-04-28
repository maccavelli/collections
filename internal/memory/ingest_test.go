package memory

import (
	"context"
	"mcp-server-recall/internal/config"
	"mcp-server-recall/internal/search"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryStore_IngestAndProcess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recall-ingest-*")
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
	// Mock bleve search engine to prevent DeleteByPath from panicking
	mockEngine := &mockSearchEngine{indexed: make(map[string]*search.Document)}
	store.SetSearchEngine(ctx, mockEngine)

	t.Run("clipMarkdown", func(t *testing.T) {
		mdContent := "# Header 1\nContent 1\n## Header 2\nContent 2"
		entries := store.clipMarkdown("test.md", mdContent, "fakem5")
		if len(entries) != 2 {
			t.Errorf("expected 2 markdown chunks, got %d", len(entries))
		}
		if entries[0].Title != "Header 1" {
			t.Errorf("expected Header 1, got %s", entries[0].Title)
		}
	})

	t.Run("clipYaml", func(t *testing.T) {
		yamlContent := "foo: bar\n---\nbaz: qux"
		entries := store.clipYaml("test.yaml", yamlContent, "fakey5")
		if len(entries) != 2 {
			t.Errorf("expected 2 yaml chunks, got %d", len(entries))
		}
	})

	t.Run("ProcessPath_SingleFile", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "test_file.txt")
		os.WriteFile(testFile, []byte("Hello, this is a plain text file."), 0644)

		entriesStored, err := store.ProcessPath(ctx, testFile)
		if err != nil {
			t.Fatalf("ProcessPath failed: %v", err)
		}
		if entriesStored != 1 {
			t.Errorf("expected 1 entry stored, got %d", entriesStored)
		}

		// Run again to test hash suppression logic
		entriesStored2, err := store.ProcessPath(ctx, testFile)
		if err != nil {
			t.Fatalf("ProcessPath 2nd run failed: %v", err)
		}
		if entriesStored2 != 0 {
			t.Errorf("expected 0 entries stored due to hash match, got %d", entriesStored2)
		}
	})

	t.Run("ProcessPath_Directory", func(t *testing.T) {
		sandboxDir := filepath.Join(tmpDir, "sandbox")
		os.MkdirAll(sandboxDir, 0755)

		os.WriteFile(filepath.Join(sandboxDir, "f1.md"), []byte("# Title\nData"), 0644)
		os.WriteFile(filepath.Join(sandboxDir, "f2.yaml"), []byte("data: 1"), 0644)
		os.WriteFile(filepath.Join(sandboxDir, "f3.json"), []byte(`{"data":1}`), 0644)

		// Write ignored directory
		vendorDir := filepath.Join(sandboxDir, "vendor")
		os.MkdirAll(vendorDir, 0755)
		os.WriteFile(filepath.Join(vendorDir, "ignored.md"), []byte("# Should not exist"), 0644)

		entriesStored, err := store.ProcessPath(ctx, sandboxDir)
		if err != nil {
			t.Fatalf("ProcessPath Dir failed: %v", err)
		}

		// 1 md chunk, 1 yaml chunk, 1 json generic chunk = 3 total. vendor should be skipped.
		if entriesStored != 3 {
			t.Errorf("expected 3 entries stored, got %d", entriesStored)
		}
	})

	t.Run("DeleteByCategory", func(t *testing.T) {
		// Mock entries into badger directly via Save since mockSearchEngine doesn't sync DB keys automatically
		// but wait DeleteByCategory scans badger using "_idx:cat:...", so it needs real saves
		_, _ = store.Save(ctx, "", "purge-key-1", "content", "purge-cat", nil, "", 0)
		_, _ = store.Save(ctx, "", "purge-key-2", "content", "purge-cat", nil, "", 0)

		deleted, err := store.DeleteByCategory(ctx, "purge-cat")
		if err != nil {
			t.Fatalf("DeleteByCategory failed: %v", err)
		}
		if deleted != 2 {
			t.Errorf("expected 2 deleted records in category, got %d", deleted)
		}

		deleted2, _ := store.DeleteByCategory(ctx, "purge-cat")
		if deleted2 != 0 {
			t.Errorf("expected 0, got %d", deleted2)
		}
	})

	t.Run("DeleteByCategory_Standards", func(t *testing.T) {
		_, err := store.DeleteByCategory(ctx, "HarvestedCode")
		if err == nil {
			t.Fatalf("expected error deleting standards category, got nil")
		}
	})
}
