package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AnthropicProvider implements the Provider interface using the Anthropic API via standard http client.
type AnthropicProvider struct {
	apiKey  string
	model   string
	baseURL string // For testing
}

// NewAnthropic creates a new Anthropic provider instance.
func NewAnthropic(apiKey, model string) (*AnthropicProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic api key is required")
	}
	return &AnthropicProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.anthropic.com/v1",
	}, nil
}

// Name returns the provider's identifier.
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// Generate sends a prompt to Anthropic's Messages API and returns the generated text.
func (p *AnthropicProvider) Generate(ctx context.Context, prompt string) (string, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":      p.model,
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic error (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("anthropic returned empty content")
	}

	return result.Content[0].Text, nil
}

// DiscoverModels tests the default Anthropic models concurrently
// and returns only the responsive ones.
func (p *AnthropicProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	candidates := []string{"claude-3-5-sonnet-latest", "claude-3-5-haiku-latest"}
	var results []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, id := range candidates {
		wg.Add(1)
		go func(modelID string) {
			defer wg.Done()
			tCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			tp, _ := NewAnthropic(p.apiKey, modelID)
			tp.baseURL = p.baseURL // inherit mock URLs in tests
			res, err := tp.Generate(tCtx, "Respond with ONLY the word Hello")
			if err == nil && strings.Contains(strings.ToLower(res), "hello") {
				mu.Lock()
				results = append(results, modelID)
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	return results, nil
}
