// Package sync provides functionality for the sync subsystem.
package sync

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/embedded"
)

var (
	// Map known URLs to their embedded file counterparts for Airgap fallback.
	embeddedMap = map[string]string{
		"https://raw.githubusercontent.com/nodejs/Release/main/README.md":                                            "standards/node/release.md",
		"https://raw.githubusercontent.com/goldbergyoni/nodebestpractices/master/sections/projectstructre/readme.md": "standards/node/structure.md",
		"https://raw.githubusercontent.com/goldbergyoni/nodebestpractices/master/sections/errorhandling/readme.md":   "standards/node/errorhandling.md",
		"https://raw.githubusercontent.com/goldbergyoni/nodebestpractices/master/sections/security/readme.md":        "standards/node/security.md",
		"https://raw.githubusercontent.com/goldbergyoni/nodebestpractices/master/sections/production/readme.md":      "standards/node/production.md",

		"https://raw.githubusercontent.com/dotnet/core/main/release-notes/8.0/README.md":                                 "standards/dotnet/release.md",
		"https://raw.githubusercontent.com/dotnet/docs/main/docs/csharp/fundamentals/coding-style/coding-conventions.md": "standards/dotnet/conventions.md",
		"https://raw.githubusercontent.com/dotnet/docs/main/docs/standard/design-guidelines/index.md":                    "standards/dotnet/design.md",
		"https://raw.githubusercontent.com/dotnet/docs/main/docs/standard/security/best-practices.md":                    "standards/dotnet/security.md",
		"https://raw.githubusercontent.com/dotnet/docs/main/docs/core/testing/unit-testing-best-practices.md":            "standards/dotnet/testing.md",

		// Fallbacks for domains and environments
		"https://raw.githubusercontent.com/magicdev/standards/main/ecommerce.md": "standards/domains/ecommerce.md",
		"https://raw.githubusercontent.com/magicdev/standards/main/erp.md":       "standards/domains/erp.md",
		"https://raw.githubusercontent.com/magicdev/standards/main/containers.md": "standards/envs/containerized.md",
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
func SyncBaselines(store *db.Store) {
	slog.Info("starting baseline standards sync cascade...")
	startTime := time.Now()

	nodeSources := viper.GetStringSlice("standards.node")
	dotnetSources := viper.GetStringSlice("standards.dotnet")
	domainSources := viper.GetStringMapStringSlice("standards.domains")
	envSources := viper.GetStringMapStringSlice("standards.environments")

	nodeAvailable := syncStack("Node", nodeSources)
	dotnetAvailable := syncStack(".NET", dotnetSources)

	// Fallbacks for domains if empty
	if len(domainSources) == 0 {
		domainSources = map[string][]string{
			"ecommerce": {"https://raw.githubusercontent.com/magicdev/standards/main/ecommerce.md"},
			"erp":       {"https://raw.githubusercontent.com/magicdev/standards/main/erp.md"},
		}
	}
	// Fallbacks for envs if empty
	if len(envSources) == 0 {
		envSources = map[string][]string{
			"containerized": {"https://raw.githubusercontent.com/magicdev/standards/main/containers.md"},
		}
	}

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

func syncStack(stack string, sources []string) []string {
	var available []string

	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source != "" {
			available = append(available, source)
		}
	}

	// Extreme Fallback: If stack is completely empty after everything, inject embedded defaults
	if len(available) == 0 {
		slog.Warn("no external standards configured for stack. injecting embedded defaults", "stack", stack)
		for src, ePath := range embeddedMap {
			isNode := strings.Contains(ePath, "/node/")
			isDotnet := strings.Contains(ePath, "/dotnet/")
			if (stack == "Node" && isNode) || (stack == ".NET" && isDotnet) {
				available = append(available, src)
			}
		}
	}

	return available
}

// GetEmbeddedStandard returns the raw content of an embedded fallback standard if available.
func GetEmbeddedStandard(url string) ([]byte, error) {
	embeddedPath, ok := embeddedMap[url]
	if !ok {
		return nil, fmt.Errorf("no embedded standard found for url: %s", url)
	}
	return embedded.BaselineFS.ReadFile(embeddedPath)
}
