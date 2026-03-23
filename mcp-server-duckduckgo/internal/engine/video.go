package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"mcp-server-duckduckgo/internal/models"
)

// VideoSearch performs a video search.
func (e *SearchEngine) VideoSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	vqd, err := e.getVQD(ctx, query)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("https://duckduckgo.com/v.js?q=%s&vqd=%s&o=json&l=us-en&p=-1", url.QueryEscape(query), vqd)
	req, err := e.newRequest(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("video search request creation failed: %w", err)
	}
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("video search failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error on read-only close is safe to ignore

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("video search failed with status code: %d", resp.StatusCode)
	}

	var data struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Duration    string `json:"duration"`
			Publisher   string `json:"publisher"`
			Published   string `json:"published"`
		} `json:"results"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode video search results: %w", err)
	}

	results := make([]models.SearchResult, 0, maxResults)
	for i, r := range data.Results {
		if i >= maxResults {
			break
		}
		results = append(results, models.SearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: truncate(r.Description, MaxSnippetLength),
			Duration:    r.Duration,
			Publisher:   r.Publisher,
			Date:        r.Published,
		})
	}
	return results, nil
}
