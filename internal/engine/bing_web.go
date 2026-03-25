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

// BingWebSearch performs a web search using Bing's public endpoint as a fallback.
func (e *SearchEngine) BingWebSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	u := fmt.Sprintf("https://www.bing.com/search?q=%s", url.QueryEscape(query))
	req, err := e.newRequest(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create bing search request: %w", err)
	}

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform bing search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bing search failed with status code: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, config.MaxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse bing search results: %w", err)
	}

	results := make([]models.SearchResult, 0, maxResults)
	doc.Find("li.b_algo").Each(func(i int, s *goquery.Selection) {
		if len(results) >= maxResults {
			return
		}

		title := strings.TrimSpace(s.Find("h2").Text())
		link, exists := s.Find("a").Attr("href")
		snippet := strings.TrimSpace(s.Find(".b_caption p, .b_snippet").Text())

		if title != "" && exists && link != "" && strings.HasPrefix(link, "http") {
			results = append(results, models.SearchResult{
				Title:       title,
				URL:         link,
				Description: truncate(snippet, config.MaxSnippetLength),
				Source:      "Bing",
			})
		}
	})

	return results, nil
}
