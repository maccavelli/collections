package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"mcp-server-duckduckgo/internal/config"
	"mcp-server-duckduckgo/internal/models"
)

// WebSearch performs a high-concurrency web search by querying DDG and Google.
// It uses the shared runProviders runner and falls back to Bing only if primaries fail.
func (e *SearchEngine) WebSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	// Standard web search providers
	providers := []providerFunc{
		func(c context.Context, q string, m int) ([]models.SearchResult, error) {
			return e.ddgWebSearch(c, q, m)
		},
		func(c context.Context, q string, m int) ([]models.SearchResult, error) {
			return e.GoogleSearch(c, q, "", m)
		},
	}

	// We deduplicate based on the URL
	dedupeKey := func(r models.SearchResult) string {
		return r.URL
	}

	results, err := e.runProviders(ctx, query, maxResults, dedupeKey, providers...)
	if err == nil {
		return results, nil
	}

	// Absolute fallback if primary providers fail
	return e.BingWebSearch(ctx, query, maxResults)
}

func (e *SearchEngine) ddgWebSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	formData := url.Values{}
	formData.Set("q", query)
	formData.Set("b", "")

	req, err := e.newRequest(ctx, http.MethodPost, "https://html.duckduckgo.com/html", strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://html.duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ddg web search failed with status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, config.MaxBodyBytes))
	if err != nil {
		return nil, err
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
				Description: truncate(snippet, config.MaxSnippetLength),
				Source:      "DuckDuckGo",
			})
		}
	})
	return results, nil
}
