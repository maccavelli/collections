package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"mcp-server-magicdev/internal/db"
)

func TestGetAvailableStandards(t *testing.T) {
	availableStandardsMu.Lock()
	availableStandards["Node"] = []string{"std1", "std2"}
	availableStandards[".NET"] = []string{"std3"}
	availableStandardsMu.Unlock()

	nodeStds := GetAvailableStandards("Node")
	if len(nodeStds) != 2 {
		t.Errorf("Expected 2 node standards, got %d", len(nodeStds))
	}

	dotnetStds := GetAvailableStandards("dotnet") // case insensitivity test
	if len(dotnetStds) != 1 || dotnetStds[0] != "std3" {
		t.Errorf("Expected 1 .NET standard, got %v", dotnetStds)
	}

	unknownStds := GetAvailableStandards("Python")
	if len(unknownStds) != 2 {
		t.Errorf("Expected 2 standards for unknown stack (fallback to Node), got %d", len(unknownStds))
	}
}

func TestSyncBaselines(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	// 1. Test successful local file sync
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test_std.md")
	os.WriteFile(tempFile, []byte("test content"), 0644)

	viper.Set("standards.node", []string{tempFile})
	viper.Set("standards.dotnet", []string{}) // empty

	SyncBaselines(store)

	nodeStds := GetAvailableStandards("Node")
	if len(nodeStds) != 1 || nodeStds[0] != tempFile {
		t.Errorf("Expected node standards to contain local file, got %v", nodeStds)
	}

	// Verify content was cached in BuntDB
	cached, err := store.GetBaselineContent(tempFile)
	if err != nil {
		t.Errorf("Expected cached content in BuntDB, got error: %v", err)
	}
	if cached != "test content" {
		t.Errorf("Expected cached content 'test content', got '%s'", cached)
	}
}

func TestSyncBaselines_CacheHit(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	// Pre-populate BuntDB with cached content
	url := "https://example.com/test-standard.md"
	err = store.SetBaseline(url, "pre-cached content", "hash123")
	if err != nil {
		t.Fatalf("Failed to pre-cache baseline: %v", err)
	}

	// SyncBaselines should use the cache and NOT attempt HTTP download
	viper.Set("standards.node", []string{url})
	viper.Set("standards.dotnet", []string{})

	SyncBaselines(store)

	nodeStds := GetAvailableStandards("Node")
	if len(nodeStds) != 1 || nodeStds[0] != url {
		t.Errorf("Expected node standards to contain cached URL, got %v", nodeStds)
	}

	// Verify BuntDB still has the pre-cached content (not overwritten)
	cached, err := store.GetBaselineContent(url)
	if err != nil {
		t.Errorf("Expected cached content, got error: %v", err)
	}
	if cached != "pre-cached content" {
		t.Errorf("Expected 'pre-cached content', got '%s'", cached)
	}
}

func TestFetchAndCache_LocalFile(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "local_standard.md")
	os.WriteFile(tempFile, []byte("local file content"), 0644)

	// First call should fetch and cache
	err = FetchAndCache(store, tempFile)
	if err != nil {
		t.Fatalf("FetchAndCache failed for local file: %v", err)
	}

	cached, err := store.GetBaselineContent(tempFile)
	if err != nil {
		t.Fatalf("Expected cached content after FetchAndCache: %v", err)
	}
	if cached != "local file content" {
		t.Errorf("Expected 'local file content', got '%s'", cached)
	}

	// Second call should be a cache hit (no re-read)
	err = FetchAndCache(store, tempFile)
	if err != nil {
		t.Fatalf("FetchAndCache cache hit should succeed: %v", err)
	}
}

func TestFetchAndCache_EmbeddedFallback(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	// Use a known embedded URL that will fail HTTP but has an embedded fallback
	knownURL := "https://raw.githubusercontent.com/nodejs/Release/main/README.md"

	// FetchAndCache should succeed (either via HTTP or embedded fallback)
	err = FetchAndCache(store, knownURL)
	if err != nil {
		t.Fatalf("FetchAndCache should succeed for known embedded URL: %v", err)
	}

	// Verify content was cached
	cached, err := store.GetBaselineContent(knownURL)
	if err != nil {
		t.Errorf("Expected cached content, got error: %v", err)
	}
	if cached == "" {
		t.Error("Expected non-empty cached content")
	}
}

func TestGetContextualStandards(t *testing.T) {
	availableStandardsMu.Lock()
	availableStandards["Node"] = []string{"std-node"}
	domainStandards["ecommerce"] = []string{"std-ecom"}
	envStandards["containerized"] = []string{"std-docker"}
	availableStandardsMu.Unlock()

	res := GetContextualStandards("Node", "containerized", []string{"ecommerce", "other"})
	if len(res) != 3 {
		t.Errorf("Expected 3 standards, got %d", len(res))
	}

	foundNode, foundEcom, foundDocker := false, false, false
	for _, r := range res {
		if r == "std-node" {
			foundNode = true
		}
		if r == "std-ecom" {
			foundEcom = true
		}
		if r == "std-docker" {
			foundDocker = true
		}
	}

	if !foundNode || !foundEcom || !foundDocker {
		t.Errorf("Missing expected standards in result: %v", res)
	}
}

func TestFetchAndCacheWithContent_LocalFile(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "content_test.md")
	os.WriteFile(tempFile, []byte("returned content"), 0644)

	// Should fetch, cache, and return content in one call.
	content, err := FetchAndCacheWithContent(store, tempFile)
	if err != nil {
		t.Fatalf("FetchAndCacheWithContent failed: %v", err)
	}
	if content != "returned content" {
		t.Errorf("Expected 'returned content', got '%s'", content)
	}

	// Verify it was cached.
	if !store.HasBaseline(tempFile) {
		t.Error("Expected baseline to be cached after FetchAndCacheWithContent")
	}
}

func TestFetchAndCacheWithContent_CacheHit(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	url := "https://example.com/cached-standard.md"
	if err := store.SetBaseline(url, "pre-cached", "hash"); err != nil {
		t.Fatalf("SetBaseline failed: %v", err)
	}

	// Should return cached content without any fetch attempt.
	content, err := FetchAndCacheWithContent(store, url)
	if err != nil {
		t.Fatalf("FetchAndCacheWithContent cache hit failed: %v", err)
	}
	if content != "pre-cached" {
		t.Errorf("Expected 'pre-cached', got '%s'", content)
	}
}
