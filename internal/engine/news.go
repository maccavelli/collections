package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"mcp-server-duckduckgo/internal/models"
)

// NewsSearch performs a news search.
func (e *SearchEngine) NewsSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	vqd, err := e.getVQD(ctx, query)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("https://duckduckgo.com/news.js?q=%s&vqd=%s&l=us-en&o=json&p=-1", url.QueryEscape(query), vqd)
	req, err := e.newRequest(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("news search request creation failed: %w", err)
	}
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("news search failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error on read-only close is safe to ignore

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("news search failed with status code: %d", resp.StatusCode)
	}

	var data struct {
		Results []struct {
			Title   string      `json:"title"`
			URL     string      `json:"url"`
			Excerpt string      `json:"excerpt"`
			Source  string      `json:"source"`
			Date    interface{} `json:"date"`
			Image   string      `json:"image"`
		} `json:"results"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode news search results: %w", err)
	}

	results := make([]models.SearchResult, 0, maxResults)
	for i, r := range data.Results {
		if i >= maxResults {
			break
		}

		dateStr := ""
		switch v := r.Date.(type) {
		case string:
			dateStr = v
		case float64:
			dateStr = time.Unix(int64(v), 0).UTC().Format(time.RFC3339)
		case int64:
			dateStr = time.Unix(v, 0).UTC().Format(time.RFC3339)
		}

		results = append(results, models.SearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: truncate(r.Excerpt, MaxSnippetLength),
			Source:      r.Source,
			Date:        dateStr,
			ImageURL:    r.Image,
		})
	}
	return results, nil
}
