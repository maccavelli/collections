package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"mcp-server-duckduckgo/internal/models"
)

// Pre-compiled VQD extraction patterns.
var vqdPatterns = []*regexp.Regexp{
	regexp.MustCompile(`vqd='([^']+)'`),
	regexp.MustCompile(`vqd="([^"]+)"`),
	regexp.MustCompile(`vqd=([^&]+)`),
}

// SearchEngine handles DDG scraping logic.
type SearchEngine struct {
	Client *http.Client
}

func NewSearchEngine() *SearchEngine {
	return &SearchEngine{
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (e *SearchEngine) getVQD(ctx context.Context, query string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://duckduckgo.com", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create VQD request: %w", err)
	}

	q := req.URL.Query()
	q.Add("q", query)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")

	resp, err := e.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform VQD request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read VQD response body: %w", err)
	}

	for _, re := range vqdPatterns {
		matches := re.FindSubmatch(body)
		if len(matches) > 1 {
			return string(matches[1]), nil
		}
	}

	return "", fmt.Errorf("could not extract vqd")
}

// WebSearch performs a web search using high-quality HTML endpoint.
func (e *SearchEngine) WebSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	formData := url.Values{}
	formData.Set("q", query)
	formData.Set("b", "")

	req, err := http.NewRequestWithContext(ctx, "POST", "https://html.duckduckgo.com/html", strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create web search request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://html.duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform web search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed with status code: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse web search results: %w", err)
	}

	results := []models.SearchResult{}
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
				Description: snippet,
			})
		}
	})

	return results, nil
}

// NewsSearch performs a news search.
func (e *SearchEngine) NewsSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	vqd, err := e.getVQD(ctx, query)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("https://duckduckgo.com/news.js?q=%s&vqd=%s&l=us-en&o=json&p=-1", url.QueryEscape(query), vqd)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("news search request creation failed: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("news search failed: %w", err)
	}
	defer resp.Body.Close()

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

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode news search results: %w", err)
	}

	results := make([]models.SearchResult, 0, len(data.Results))
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
			Description: r.Excerpt,
			Source:      r.Source,
			Date:        dateStr,
			ImageURL:    r.Image,
		})
	}
	return results, nil
}

// ImageSearch performs an image search.
func (e *SearchEngine) ImageSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	vqd, err := e.getVQD(ctx, query)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("https://duckduckgo.com/i.js?q=%s&vqd=%s&o=json&l=us-en&p=1", url.QueryEscape(query), vqd)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("image search request creation failed: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image search failed: %w", err)
	}
	defer resp.Body.Close()

	var data struct {
		Results []struct {
			Title     string `json:"title"`
			Image     string `json:"image"`
			Thumbnail string `json:"thumbnail"`
			URL       string `json:"url"`
			Source    string `json:"source"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode image search results: %w", err)
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

// VideoSearch performs a video search.
func (e *SearchEngine) VideoSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	vqd, err := e.getVQD(ctx, query)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("https://duckduckgo.com/v.js?q=%s&vqd=%s&o=json&l=us-en&p=-1", url.QueryEscape(query), vqd)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("video search request creation failed: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("video search failed: %w", err)
	}
	defer resp.Body.Close()

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

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode video search results: %w", err)
	}

	results := make([]models.SearchResult, 0, len(data.Results))
	for i, r := range data.Results {
		if i >= maxResults {
			break
		}
		results = append(results, models.SearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
			Duration:    r.Duration,
			Publisher:   r.Publisher,
			Date:        r.Published,
		})
	}
	return results, nil
}
