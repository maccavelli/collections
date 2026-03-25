package engine

import (
	"context"
	"fmt"
	"mcp-server-duckduckgo/internal/config"
	"mcp-server-duckduckgo/internal/models"
)

// providerResult is an internal type for gathering search results from different engines.
type providerResult struct {
	data []models.SearchResult
	err  error
}

// providerFunc is a standard function signature for search providers.
type providerFunc func(context.Context, string, int) ([]models.SearchResult, error)

// runProviders executes multiple search providers in parallel, merges results, and deduplicates them.
func (e *SearchEngine) runProviders(
	ctx context.Context,
	query string,
	maxResults int,
	dedupeKey func(models.SearchResult) string,
	providers ...providerFunc,
) ([]models.SearchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, config.DefaultTimeout)
	defer cancel()

	resChan := make(chan providerResult, len(providers))

	for _, p := range providers {
		go func(p providerFunc) {
			data, err := p(ctx, query, maxResults)
			select {
			case resChan <- providerResult{data, err}:
			case <-ctx.Done():
			}
		}(p)
	}

	merged := make([]models.SearchResult, 0, maxResults)
	seen := make(map[string]bool)
	received := 0

	for received < len(providers) {
		select {
		case res := <-resChan:
			received++
			if res.err == nil {
				for _, r := range res.data {
					key := dedupeKey(r)
					if key != "" {
						if !seen[key] {
							seen[key] = true
							merged = append(merged, r)
							if len(merged) >= maxResults {
								return merged, nil
							}
						}
					} else {
						// For search types without a dedupe key (e.g. books), just append
						merged = append(merged, r)
						if len(merged) >= maxResults {
							return merged, nil
						}
					}
				}
			}
		case <-ctx.Done():
			if len(merged) > 0 {
				return merged, nil
			}
			return nil, ctx.Err()
		}
	}

	if len(merged) > 0 {
		return merged, nil
	}

	return nil, fmt.Errorf("no results found across %d providers", len(providers))
}
