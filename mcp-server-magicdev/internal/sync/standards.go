// Package sync provides functionality for the sync subsystem.
package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/embedded"
)

var (
	// embeddedMap maps known URLs to their embedded file counterparts for Airgap fallback.
	embeddedMap = map[string]string{
		// Node.js standards
		"https://raw.githubusercontent.com/nodejs/Release/main/README.md":            "standards/node/release.md",
		"https://raw.githubusercontent.com/goldbergyoni/nodebestpractices/master/README.md": "standards/node/bestpractices.md",
		"https://raw.githubusercontent.com/nodejs/docker-node/main/docs/BestPractices.md":  "standards/node/docker-bestpractices.md",
		"https://raw.githubusercontent.com/nodejs/docker-node/main/README.md":              "standards/node/docker-readme.md",

		// .NET standards
		"https://raw.githubusercontent.com/dotnet/core/main/release-notes/8.0/README.md":                                 "standards/dotnet/release.md",
		"https://raw.githubusercontent.com/dotnet/docs/main/docs/csharp/fundamentals/coding-style/coding-conventions.md": "standards/dotnet/conventions.md",
		"https://raw.githubusercontent.com/dotnet/docs/main/docs/standard/design-guidelines/index.md":                    "standards/dotnet/design.md",
		"https://raw.githubusercontent.com/dotnet/docs/main/docs/standard/security/secure-coding-guidelines.md":          "standards/dotnet/security.md",
		"https://raw.githubusercontent.com/dotnet/docs/main/docs/core/testing/unit-testing-best-practices.md":            "standards/dotnet/testing.md",
		"https://raw.githubusercontent.com/dotnet/dotnet-docker/main/samples/README.md":                                  "standards/dotnet/docker-samples.md",
		"https://raw.githubusercontent.com/dotnet/dotnet-docker/main/documentation/scenarios/installing-dotnet.md":       "standards/dotnet/docker-install.md",
	}

	availableStandardsMu sync.RWMutex
	availableStandards   = map[string][]string{
		".NET": {},
		"Node": {},
	}
	domainStandards = make(map[string][]string)
	envStandards    = make(map[string][]string)
)

// GetContextualStandards aggregates stack, environment, and domain standards.
func GetContextualStandards(stack, environment string, labels []string) []string {
	availableStandardsMu.RLock()
	defer availableStandardsMu.RUnlock()

	stackKey := "Node"
	if strings.Contains(strings.ToLower(stack), "net") {
		stackKey = ".NET"
	}

	// 1. Get base stack standards
	var aggregated []string
	if list, exists := availableStandards[stackKey]; exists {
		aggregated = append(aggregated, list...)
	}

	// 2. Add environmental standards
	envKey := strings.ToLower(strings.TrimSpace(environment))
	if envKey != "" {
		if list, exists := envStandards[envKey]; exists {
			aggregated = append(aggregated, list...)
		}
	}

	// 3. Add domain standards based on labels
	for _, label := range labels {
		lblKey := strings.ToLower(strings.TrimSpace(label))
		if list, exists := domainStandards[lblKey]; exists {
			aggregated = append(aggregated, list...)
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var final []string
	for _, url := range aggregated {
		if !seen[url] {
			seen[url] = true
			final = append(final, url)
		}
	}

	return final
}

// GetAvailableStandards returns the finalized list of standards that survived the sync cascade.
func GetAvailableStandards(stack string) []string {
	availableStandardsMu.RLock()
	defer availableStandardsMu.RUnlock()
	// Stack could be lowercased or varied, standardize it for lookup
	stackKey := "Node"
	if strings.Contains(strings.ToLower(stack), "net") {
		stackKey = ".NET"
	}

	list, exists := availableStandards[stackKey]
	if !exists {
		return []string{}
	}
	return list
}

// SyncBaselines iterates over both remote URLs and local directory paths to populate
// the available standards list. Local .md files found in the configured path directories
// are pre-loaded into BuntDB with hash-aware caching alongside URL-based standards.
func SyncBaselines(store *db.Store) {
	slog.Info("starting baseline standards sync cascade...")
	startTime := time.Now()

	nodeSources := viper.GetStringSlice("standards.node.urls")
	dotnetSources := viper.GetStringSlice("standards.dotnet.urls")
	domainSources := viper.GetStringMapStringSlice("standards.domains")
	envSources := viper.GetStringMapStringSlice("standards.environments")

	// Append local filesystem standards from the configured path directories.
	// These are the embedded standards extracted to the OS cache at init time.
	nodeSources = appendLocalStandards(nodeSources, viper.GetString("standards.node.path"))
	dotnetSources = appendLocalStandards(dotnetSources, viper.GetString("standards.dotnet.path"))

	nodeAvailable := syncStack("Node", nodeSources, store)
	dotnetAvailable := syncStack(".NET", dotnetSources, store)

	availableStandardsMu.Lock()
	availableStandards["Node"] = nodeAvailable
	availableStandards[".NET"] = dotnetAvailable

	for k, v := range domainSources {
		domainStandards[strings.ToLower(k)] = v
	}
	for k, v := range envSources {
		envStandards[strings.ToLower(k)] = v
	}
	availableStandardsMu.Unlock()

	slog.Info("baseline standards sync complete", "duration", time.Since(startTime))
}

// appendLocalStandards scans the given directory for .md files and appends their
// absolute paths to the sources list. Skips hidden files, duplicates, and non-.md files.
func appendLocalStandards(sources []string, dirPath string) []string {
	if dirPath == "" {
		return sources
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		slog.Debug("standards directory not found, skipping local scan",
			"dir", dirPath,
			"error", err,
		)
		return sources
	}

	// Build a set of existing sources to avoid duplicates.
	seen := make(map[string]bool, len(sources))
	for _, s := range sources {
		seen[s] = true
	}

	var added int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
			continue
		}

		absPath := filepath.Join(dirPath, name)
		if seen[absPath] {
			continue
		}

		sources = append(sources, absPath)
		seen[absPath] = true
		added++
	}

	if added > 0 {
		slog.Info("discovered local standards files",
			"dir", dirPath,
			"count", added,
		)
	}

	return sources
}

// syncStack processes a list of standard source URLs for a given stack.
// For each URL, it follows the BuntDB-first cascade: check cache, then HTTP, then embedded fallback.
func syncStack(stack string, sources []string, store *db.Store) []string {
	var available []string

	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}

		if err := FetchAndCache(store, source); err != nil {
			slog.Warn("failed to fetch or cache standard",
				"stack", stack,
				"source", source,
				"error", err,
			)
			// Still add to available list — the content may be fetched
			// on-demand by IngestStandards if the network recovers.
		}
		available = append(available, source)
	}

	if len(available) == 0 {
		slog.Warn("no standards configured for stack", "stack", stack)
	}

	return available
}
// FetchAndCache implements the standards retrieval cascade. For local filesystem
// sources, it uses SHA-256 hash comparison to avoid unnecessary BuntDB writes.
// For remote URLs, it uses the existing TTL-based existence check.
//
// Local file cascade:
//  1. Compute SHA-256 of on-disk file
//  2. Compare to BuntDB stored hash — skip if match
//  3. Re-read and re-cache if hash differs or entry missing
//
// URL cascade:
//  1. BuntDB cache (TTL-based existence check)
//  2. HTTP download (only if not cached)
//  3. Embedded fallback (only if HTTP fails)
func FetchAndCache(store *db.Store, source string) error {
	isURL := strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")

	if !isURL {
		return fetchAndCacheLocal(store, source)
	}

	return fetchAndCacheURL(store, source)
}

// fetchAndCacheLocal handles local filesystem standards with hash-aware caching.
// It computes the SHA-256 of the on-disk file and compares to the BuntDB stored
// hash, only re-caching if the content has actually changed.
func fetchAndCacheLocal(store *db.Store, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("local standard read failed, attempting embedded fallback",
			"source", path,
			"error", err,
		)
		// Try embedded fallback for airgap environments.
		content, embedErr := GetEmbeddedStandard(path)
		if embedErr != nil {
			return fmt.Errorf("local read failed and no embedded fallback for %s: read=%v, embedded=%v", path, err, embedErr)
		}
		return storeBaseline(store, path, content)
	}

	if len(content) == 0 {
		return fmt.Errorf("local standard is empty: %s", path)
	}

	diskHash := sha256Hex(content)
	cachedHash, _ := store.GetBaselineHash(path)

	if diskHash == cachedHash {
		slog.Debug("local standard cache hit (hash match)", "source", path)
		return nil
	}

	if cachedHash == "" {
		slog.Info("local standard not cached, loading into BuntDB",
			"source", path,
			"hash", diskHash[:8],
			"bytes", len(content),
		)
	} else {
		slog.Info("local standard changed on disk, updating BuntDB cache",
			"source", path,
			"old_hash", cachedHash[:8],
			"new_hash", diskHash[:8],
			"bytes", len(content),
		)
	}

	return storeBaseline(store, path, content)
}

// fetchAndCacheURL handles remote URL standards with TTL-based existence check.
func fetchAndCacheURL(store *db.Store, source string) error {
	// Tier 1: BuntDB cache probe — zero decompression, zero unmarshal.
	if store.HasBaseline(source) {
		slog.Debug("standards cache hit, skipping download", "source", source)
		return nil
	}

	slog.Info("standards cache miss, attempting retrieval", "source", source)

	// Tier 2: HTTP download.
	content, fetchErr := httpFetch(source)
	if fetchErr == nil && len(content) > 0 {
		return storeBaseline(store, source, content)
	}

	slog.Warn("direct fetch failed, attempting embedded fallback",
		"source", source,
		"error", fetchErr,
	)

	// Tier 3: Embedded fallback for airgap environments.
	content, embedErr := GetEmbeddedStandard(source)
	if embedErr != nil {
		return fmt.Errorf("all retrieval tiers exhausted for %s: fetch=%v, embedded=%v", source, fetchErr, embedErr)
	}

	return storeBaseline(store, source, content)
}

// sha256Hex computes the SHA-256 hash of data and returns it as a hex string.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// FetchAndCacheWithContent is identical to FetchAndCache but returns the
// decompressed content on success. This eliminates the need for callers
// to re-read from BuntDB after caching, avoiding a double decompression.
func FetchAndCacheWithContent(store *db.Store, source string) (string, error) {
	// Tier 1: BuntDB cache — read and return content directly.
	if cached, err := store.GetBaselineContent(source); err == nil && cached != "" {
		slog.Debug("standards cache hit", "source", source)
		return cached, nil
	}

	slog.Info("standards cache miss, attempting retrieval", "source", source)

	isURL := strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")

	// Tier 2: Fetch content (HTTP or local file).
	var content []byte
	var fetchErr error

	if isURL {
		content, fetchErr = httpFetch(source)
	} else {
		content, fetchErr = os.ReadFile(source)
	}

	if fetchErr == nil && len(content) > 0 {
		if err := storeBaseline(store, source, content); err != nil {
			return "", err
		}
		return string(content), nil
	}

	slog.Warn("direct fetch failed, attempting embedded fallback",
		"source", source,
		"error", fetchErr,
	)

	// Tier 3: Embedded fallback.
	content, embedErr := GetEmbeddedStandard(source)
	if embedErr != nil {
		return "", fmt.Errorf("all retrieval tiers exhausted for %s: fetch=%v, embedded=%v", source, fetchErr, embedErr)
	}

	if err := storeBaseline(store, source, content); err != nil {
		return "", err
	}
	return string(content), nil
}

// httpFetch performs an HTTP GET and returns the response body.
func httpFetch(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d for %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}

// storeBaseline computes a SHA-256 hash and persists content to BuntDB with a 30-day TTL.
func storeBaseline(store *db.Store, source string, content []byte) error {
	hashBytes := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hashBytes[:])

	if err := store.SetBaseline(source, string(content), hashStr); err != nil {
		return fmt.Errorf("failed to cache standard in BuntDB: %w", err)
	}

	slog.Info("standard cached successfully", "source", source, "bytes", len(content))
	return nil
}

// GetEmbeddedStandard returns the raw content of an embedded fallback standard if available.
func GetEmbeddedStandard(url string) ([]byte, error) {
	embeddedPath, ok := embeddedMap[url]
	if !ok {
		return nil, fmt.Errorf("no embedded standard found for url: %s", url)
	}
	return embedded.BaselineFS.ReadFile(embeddedPath)
}
