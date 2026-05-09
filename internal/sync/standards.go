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

// SyncBaselines iterates over the config URLs to populate the available standards list.
// It follows a BuntDB-first caching strategy: standards are only downloaded when they
// are not already cached in BuntDB (or have expired past the 30-day TTL).
func SyncBaselines(store *db.Store) {
	slog.Info("starting baseline standards sync cascade...")
	startTime := time.Now()

	nodeSources := viper.GetStringSlice("standards.node")
	dotnetSources := viper.GetStringSlice("standards.dotnet")
	domainSources := viper.GetStringMapStringSlice("standards.domains")
	envSources := viper.GetStringMapStringSlice("standards.environments")

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

// FetchAndCache implements the 3-tier standards retrieval cascade:
//  1. BuntDB cache (single source of truth, 30-day TTL)
//  2. HTTP download (only if not cached)
//  3. Embedded fallback (only if HTTP fails)
//
// Content is always stored back into BuntDB with a 30-day TTL on successful retrieval.
// Uses HasBaseline for zero-decompression existence check.
func FetchAndCache(store *db.Store, source string) error {
	// Tier 1: BuntDB cache probe — zero decompression, zero unmarshal.
	if store.HasBaseline(source) {
		slog.Debug("standards cache hit, skipping download", "source", source)
		return nil
	}

	slog.Info("standards cache miss, attempting retrieval", "source", source)

	// Determine if this is a URL or local file path.
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
