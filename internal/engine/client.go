package engine

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"time"

	"mcp-server-duckduckgo/internal/config"

	"golang.org/x/sync/singleflight"
)

// Pre-compiled VQD extraction patterns.
var vqdPatterns = []*regexp.Regexp{
	regexp.MustCompile(`vqd='([^']+)'`),
	regexp.MustCompile(`vqd="([^"]+)"`),
	regexp.MustCompile(`vqd=([^&]+)`),
}

type cacheEntry struct {
	vqd       string
	expiresAt time.Time
}

// SearchEngine handles DDG scraping logic.
type SearchEngine struct {
	Client   *http.Client
	vqdCache map[string]cacheEntry
	mu       sync.RWMutex
	vqdSF    singleflight.Group
}

// NewSearchEngine initializes an optimized HTTP client for search engine scraping.
func NewSearchEngine() *SearchEngine {
	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	return &SearchEngine{
		Client: &http.Client{
			Timeout:   config.DefaultTimeout,
			Transport: transport,
		},
		vqdCache: make(map[string]cacheEntry),
	}
}

// truncate safely trims a string to a maximum length of runes and adds an ellipsis.
func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "..."
}

// newRequest creates an HTTP request with common DDG headers.
func (e *SearchEngine) newRequest(ctx context.Context, method, u string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", config.UserAgent)
	return req, nil
}

func (e *SearchEngine) getVQD(ctx context.Context, query string) (string, error) {
	// 1. Initial Cache Check (Fast Path)
	e.mu.RLock()
	entry, ok := e.vqdCache[query]
	e.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		slog.Debug("VQD cache hit", "query", query)
		return entry.vqd, nil
	}

	// 2. Use singleflight to prevent redundant VQD fetches for the same query
	v, err, shared := e.vqdSF.Do(query, func() (interface{}, error) {
		// Re-check cache inside singleflight to avoid racing
		e.mu.RLock()
		entry, ok := e.vqdCache[query]
		e.mu.RUnlock()
		if ok && time.Now().Before(entry.expiresAt) {
			return entry.vqd, nil
		}

		slog.Info("VQD cache miss; fetching new token", "query", query)
		var resp *http.Response
		var doErr error
		backoff := 1 * time.Second

		for i := 0; i < 3; i++ {
			reqCtx, reqCancel := context.WithTimeout(ctx, 15*time.Second)
			req, err := e.newRequest(reqCtx, http.MethodGet, "https://duckduckgo.com", http.NoBody)
			if err != nil {
				reqCancel()
				return "", fmt.Errorf("failed to create VQD request: %w", err)
			}

			q := req.URL.Query()
			q.Add("q", query)
			req.URL.RawQuery = q.Encode()

			resp, doErr = e.Client.Do(req)
			if doErr == nil {
				reqCancel()
				break
			}
			reqCancel()

			slog.Warn("VQD request failed; retrying", "attempt", i+1, "error", doErr)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
			}
		}

		if doErr != nil {
			return "", fmt.Errorf("failed to perform VQD request after retries: %w", doErr)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("vqd fetch failed with status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxBodyBytes))
		if err != nil {
			return "", fmt.Errorf("failed to read VQD response body: %w", err)
		}

		for _, re := range vqdPatterns {
			matches := re.FindSubmatch(body)
			if len(matches) > 1 {
				vqd := string(matches[1])
				// Update Cache
				e.mu.Lock()
				if len(e.vqdCache) >= config.VQDCacheLimit {
					slog.Warn("VQD cache limit reached; flushing", "limit", config.VQDCacheLimit)
					e.vqdCache = make(map[string]cacheEntry)
				}
				e.vqdCache[query] = cacheEntry{
					vqd:       vqd,
					expiresAt: time.Now().Add(config.VQDCacheTTL),
				}
				e.mu.Unlock()
				return vqd, nil
			}
		}

		return "", fmt.Errorf("could not extract vqd")
	})

	if err != nil {
		return "", err
	}

	if shared {
		slog.Debug("VQD fetch shared across concurrent requests", "query", query)
	}

	return v.(string), nil
}
