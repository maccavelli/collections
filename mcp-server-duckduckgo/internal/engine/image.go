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

// ImageSearch performs a high-concurrency image search across multiple providers.
func (e *SearchEngine) ImageSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	providers := []providerFunc{
		func(c context.Context, q string, m int) ([]models.SearchResult, error) {
			return e.ddgImageSearch(c, q, m)
		},
		func(c context.Context, q string, m int) ([]models.SearchResult, error) {
			return e.GoogleSearch(c, q, "isch", m)
		},
		func(c context.Context, q string, m int) ([]models.SearchResult, error) {
			return e.BingImageSearch(c, q, m)
		},
	}

	dedupeKey := func(r models.SearchResult) string {
		return r.ImageURL
	}

	return e.runProviders(ctx, query, maxResults, dedupeKey, providers...)
}

func (e *SearchEngine) ddgImageSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	vqd, err := e.getVQD(ctx, query)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("https://duckduckgo.com/i.js?q=%s&vqd=%s&o=json&l=us-en&p=1", url.QueryEscape(query), vqd)
	req, err := e.newRequest(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ddg image search failed with status %d", resp.StatusCode)
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
		return nil, err
	}

	results := make([]models.SearchResult, 0, len(data.Results))
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
