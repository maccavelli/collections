package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"mcp-server-duckduckgo/internal/config"
	"mcp-server-duckduckgo/internal/models"
)

// ImageSearch performs an image search.
func (e *SearchEngine) ImageSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	vqd, err := e.getVQD(ctx, query)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("https://duckduckgo.com/i.js?q=%s&vqd=%s&o=json&l=us-en&p=1", url.QueryEscape(query), vqd)
	req, err := e.newRequest(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("image search request creation failed: %w", err)
	}
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image search failed with status code: %d", resp.StatusCode)
	}

	var data struct {
		Results []struct {
			Title     string `json:"title"`
			Image     string `json:"image"`
			Thumbnail string `json:"thumbnail"`
			URL       string `json:"url"`
			Source    string `json:"source"`
		} `json:"results"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, config.MaxBodyBytes)).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode image search results: %w", err)
	}

	results := make([]models.SearchResult, 0, maxResults)
	for i, r := range data.Results {
		if i >= maxResults {
			break
		}
		results = append(results, models.SearchResult{
			Title:     r.Title,
			URL:       r.URL,
			ImageURL:  r.Image,
			Thumbnail: r.Thumbnail,
			Source:    r.Source,
		})
	}
	return results, nil
}
