package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"mcp-server-duckduckgo/internal/models"
)

// WebSearch performs a web search using high-quality HTML endpoint.
func (e *SearchEngine) WebSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	formData := url.Values{}
	formData.Set("q", query)
	formData.Set("b", "")

	req, err := e.newRequest(ctx, http.MethodPost, "https://html.duckduckgo.com/html", strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create web search request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://html.duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform web search: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error on read-only close is safe to ignore

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed with status code: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse web search results: %w", err)
	}

	results := make([]models.SearchResult, 0, maxResults)
	doc.Find(".web-result").Each(func(i int, s *goquery.Selection) {
		if len(results) >= maxResults {
			return
		}
		title := strings.TrimSpace(s.Find(".result__title").Text())
		link, exists := s.Find(".result__a").Attr("href")
		snippet := strings.TrimSpace(s.Find(".result__snippet").Text())

		if title != "" && exists && link != "" {
			results = append(results, models.SearchResult{
				Title:       title,
				URL:         link,
				Description: truncate(snippet, MaxSnippetLength),
			})
		}
	})

	return results, nil
}
