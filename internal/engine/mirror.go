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

// BookSearch performs a book search using Anna's Archive for functional parity.
// It queries multiple mirrors concurrently for improved performance and resilience.
func (e *SearchEngine) BookSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	mirrors := []string{"gd", "gl", "pk"}

	type result struct {
		data []models.SearchResult
		err  error
	}

	resChan := make(chan result, len(mirrors))
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, tld := range mirrors {
		go func(tld string) {
			data, err := e.queryMirror(childCtx, tld, query, maxResults)
			select {
			case resChan <- result{data, err}:
				if err == nil && len(data) > 0 {
					cancel() // Stop other requests if we found results
				}
			case <-childCtx.Done():
				// Already finished or cancelled
			}
		}(tld)
	}

	var lastErr error
	for i := 0; i < len(mirrors); i++ {
		select {
		case res := <-resChan:
			if res.err == nil && len(res.data) > 0 {
				return res.data, nil
			}
			if res.err != nil {
				lastErr = res.err
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("book search failed across all mirrors: %w", lastErr)
	}
	return nil, fmt.Errorf("book search returned no results from any mirror")
}

// queryMirror performs a search against a specific Anna's Archive mirror.
func (e *SearchEngine) queryMirror(ctx context.Context, tld, query string, maxResults int) ([]models.SearchResult, error) {
	baseUrl := fmt.Sprintf("https://annas-archive.%s", tld)
	u := fmt.Sprintf("%s/search?q=%s", baseUrl, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mirror %s returned status %d", tld, resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Anna's Archive hides results in comments, need to strip them
	htmlStr := strings.ReplaceAll(string(respBody), "<!--", "")
	htmlStr = strings.ReplaceAll(htmlStr, "-->", "")

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return nil, err
	}

	results := []models.SearchResult{}
	doc.Find("div.js-aarecord-list-outer div.flex").Each(func(i int, s *goquery.Selection) {
		if len(results) >= maxResults {
			return
		}

		anchor := s.Find("a.text-lg, a.font-semibold, a.js-vim-focus").First()
		if anchor.Length() == 0 {
			anchor = s.Find("a").First()
		}
		title := strings.TrimSpace(anchor.Text())

		link, exists := anchor.Attr("href")
		if !exists || link == "" {
			link, _ = s.Find("a").First().Attr("href")
		}

		author := ""
		s.Find("a").Each(func(i int, sel *goquery.Selection) {
			if sel.Find(".icon-\\[mdi--user-edit\\]").Length() > 0 {
				author = strings.TrimSpace(sel.Text())
			}
		})
		if author == "" {
			author = strings.TrimSpace(s.Find("div.italic").Text())
		}

		// Extract info and description
		var infoLine string
		var description string

		s.Find("div.text-gray-800, div.text-sm.text-gray-600").Each(func(i int, sel *goquery.Selection) {
			sel.Find("script, style").Remove()
			txt := strings.TrimSpace(sel.Text())
			if txt == "" {
				return
			}
			if strings.Contains(txt, "·") && infoLine == "" {
				infoLine = txt
			} else if description == "" || len(txt) > len(description) {
				description = txt
			}
		})

		if title != "" && link != "" {
			href := link
			if !strings.HasPrefix(href, "http") {
				href = baseUrl + href
			}

			isDuplicate := false
			for _, r := range results {
				if r.URL == href {
					isDuplicate = true
					break
				}
			}

			if !isDuplicate {
				results = append(results, models.SearchResult{
					Title:       title,
					URL:         href,
					Description: description,
					Author:      author,
					Info:        infoLine,
				})
			}
		}
	})

	return results, nil
}
