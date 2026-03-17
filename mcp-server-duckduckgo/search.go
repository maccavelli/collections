package main

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
)

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

func (e *SearchEngine) getVQD(query string) (string, error) {
	req, err := http.NewRequest("GET", "https://duckduckgo.com", nil)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	q.Add("q", query)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")

	resp, err := e.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Patterns for VQD extraction
	patterns := []string{
		`vqd='([^']+)'`,
		`vqd="([^"]+)"`,
		`vqd=([^&]+)`,
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p)
		matches := re.FindSubmatch(body)
		if len(matches) > 1 {
			return string(matches[1]), nil
		}
	}

	return "", fmt.Errorf("could not extract vqd")
}

// WebSearch performs a web search using high-quality HTML endpoint.
func (e *SearchEngine) WebSearch(query string, maxResults int) ([]SearchResult, error) {
	formData := url.Values{}
	formData.Set("q", query)
	formData.Set("b", "")

	req, err := http.NewRequest("POST", "https://html.duckduckgo.com/html", strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://html.duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed with status code: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	results := []SearchResult{}
	doc.Find(".web-result").Each(func(i int, s *goquery.Selection) {
		if len(results) >= maxResults {
			return
		}
		title := s.Find(".result__title").Text()
		link, _ := s.Find(".result__a").Attr("href")
		snippet := s.Find(".result__snippet").Text()

		if title != "" && link != "" {
			results = append(results, SearchResult{
				Title:       strings.TrimSpace(title),
				URL:         link,
				Description: strings.TrimSpace(snippet),
				Type:        "web",
			})
		}
	})

	return results, nil
}

// NewsSearch performs a news search.
func (e *SearchEngine) NewsSearch(query string, maxResults int) ([]SearchResult, error) {
	vqd, err := e.getVQD(query)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("https://duckduckgo.com/news.js?q=%s&vqd=%s&l=us-en&o=json&p=-1", url.QueryEscape(query), vqd)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	results := make([]SearchResult, 0, len(data.Results))
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

		results = append(results, SearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Excerpt,
			Source:      r.Source,
			Date:        dateStr,
			ImageURL:    r.Image,
			Type:        "news",
		})
	}
	return results, nil
}

// ImageSearch performs an image search.
func (e *SearchEngine) ImageSearch(query string, maxResults int) ([]SearchResult, error) {
	vqd, err := e.getVQD(query)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("https://duckduckgo.com/i.js?q=%s&vqd=%s&o=json&l=us-en&p=1", url.QueryEscape(query), vqd)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	results := make([]SearchResult, 0, len(data.Results))
	for i, r := range data.Results {
		if i >= maxResults {
			break
		}
		results = append(results, SearchResult{
			Title:       r.Title,
			URL:         r.URL,
			ImageURL:    r.Image,
			Thumbnail:   r.Thumbnail,
			Source:      r.Source,
			Description: fmt.Sprintf("Image from %s", r.Source),
			Type:        "image",
		})
	}
	return results, nil
}

// VideoSearch performs a video search.
func (e *SearchEngine) VideoSearch(query string, maxResults int) ([]SearchResult, error) {
	vqd, err := e.getVQD(query)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("https://duckduckgo.com/v.js?q=%s&vqd=%s&o=json&l=us-en&p=-1", url.QueryEscape(query), vqd)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	results := make([]SearchResult, 0, len(data.Results))
	for i, r := range data.Results {
		if i >= maxResults {
			break
		}
		results = append(results, SearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
			Duration:    r.Duration,
			Publisher:   r.Publisher,
			Date:        r.Published,
			Type:        "video",
		})
	}
	return results, nil
}

// BookSearch performs a book search using Anna's Archive for functional parity.
// It queries multiple mirrors concurrently for improved performance and resilience.
func (e *SearchEngine) BookSearch(query string, maxResults int) ([]SearchResult, error) {
	mirrors := []string{"gd", "gl", "pk"}
	
	type result struct {
		data []SearchResult
		err  error
	}

	resChan := make(chan result, len(mirrors))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tld := range mirrors {
		go func(tld string) {
			data, err := e.queryMirror(ctx, tld, query, maxResults)
			select {
			case resChan <- result{data, err}:
				if err == nil && len(data) > 0 {
					cancel() // Stop other requests if we found results
				}
			case <-ctx.Done():
				// Already finished or cancelled
			}
		}(tld)
	}

	var lastErr error
	for i := 0; i < len(mirrors); i++ {
		res := <-resChan
		if res.err == nil && len(res.data) > 0 {
			return res.data, nil
		}
		if res.err != nil {
			lastErr = res.err
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("book search failed across all mirrors: %w", lastErr)
	}
	return nil, fmt.Errorf("book search returned no results from any mirror")
}

// queryMirror performs a search against a specific Anna's Archive mirror.
func (e *SearchEngine) queryMirror(ctx context.Context, tld, query string, maxResults int) ([]SearchResult, error) {
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

	results := []SearchResult{}
	doc.Find("div.js-aarecord-list-outer div.flex").Each(func(i int, s *goquery.Selection) {
		if len(results) >= maxResults {
			return
		}

		anchor := s.Find("a.text-lg, a.font-semibold, a.js-vim-focus").First()
		if anchor.Length() == 0 {
			anchor = s.Find("a").First()
		}
		title := anchor.Text()

		link, _ := anchor.Attr("href")
		if link == "" {
			link, _ = s.Find("a").First().Attr("href")
		}

		author := ""
		s.Find("a").Each(func(i int, sel *goquery.Selection) {
			if sel.Find(".icon-\\[mdi--user-edit\\]").Length() > 0 {
				author = sel.Text()
			}
		})
		if author == "" {
			author = s.Find("div.italic").Text()
		}

		// Extract info and metadata
		var metadataLine string
		var description string
		
		s.Find("div.text-gray-800, div.text-sm.text-gray-600").Each(func(i int, sel *goquery.Selection) {
			sel.Find("script, style").Remove()
			txt := strings.TrimSpace(sel.Text())
			if txt == "" {
				return
			}
			// Metadata line usually contains the separator "·"
			if strings.Contains(txt, "·") && metadataLine == "" {
				metadataLine = txt
			} else if description == "" || len(txt) > len(description) {
				// Pick the longest text block that isn't the metadata as the description
				description = txt
			}
		})

		metadata := make(map[string]string)
		if metadataLine != "" {
			parts := strings.Split(metadataLine, "·")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				lowerP := strings.ToLower(p)
				if (strings.Contains(p, "MB") || strings.Contains(p, "KB") || strings.Contains(p, "GB")) && metadata["size"] == "" {
					metadata["size"] = p
				} else if (strings.Contains(lowerP, "pdf") || strings.Contains(lowerP, "epub") || strings.Contains(lowerP, "mobi") || strings.Contains(lowerP, "azw")) && metadata["format"] == "" {
					metadata["format"] = p
				} else if len(p) == 4 && metadata["year"] == "" && isNumeric(p) {
					metadata["year"] = p
				} else if (strings.Contains(p, "[") && strings.Contains(p, "]") || strings.Contains(lowerP, "english") || strings.Contains(lowerP, "chinese")) && metadata["language"] == "" {
					metadata["language"] = p
				} else {
					if metadata["category"] == "" {
						metadata["category"] = p
					} else {
						metadata["category"] += " · " + p
					}
				}
			}
		}

		if title != "" && link != "" {
			title = strings.TrimSpace(title)
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
				// Fallback description if only metadata was found
				if description == "" {
					description = metadataLine
				}
				
				results = append(results, SearchResult{
					Title:       title,
					URL:         href,
					Description: description,
					Author:      strings.TrimSpace(author),
					Type:        "book",
					Metadata:    metadata,
				})
			}
		}
	})

	return results, nil
}

func isNumeric(s string) bool {
for _, r := range s {
if r < '0' || r > '9' {
return false
}
}
return true
}
