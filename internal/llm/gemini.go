package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// GeminiProvider implements Provider using the Google Gemini API via standard http client.
type GeminiProvider struct {
	apiKey  string
	model   string
	baseURL string // For testing
}

// NewGemini creates a Gemini provider with the given API key and model.
func NewGemini(ctx context.Context, apiKey, model string) (*GeminiProvider, error) {
	return &GeminiProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
	}, nil
}

// Name returns the provider's unique identifier "gemini".
func (p *GeminiProvider) Name() string { return "gemini" }

// Generate generates a commit message using the Gemini API based on the provided prompt string.
func (p *GeminiProvider) Generate(ctx context.Context, prompt string) (string, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
	})

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.baseURL, p.model, p.apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini error (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no content")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

// DiscoverModels fetches available models from the Gemini API and returns a curated selection (1 Pro, 2 Flash).
func (p *GeminiProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/models?key=%s", p.baseURL, p.apiKey)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini models error (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name                       string   `json:"name"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var candidates []string
	for _, m := range result.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		sm := strings.ToLower(id)

		// FAST FILTERING: Only consider Gemini family, skip previews/vision/image/non-gemini
		if !strings.HasPrefix(sm, "gemini") {
			continue
		}
		if strings.Contains(sm, "image") || strings.Contains(sm, "vision") {
			continue
		}
		if strings.Contains(sm, "preview") || strings.Contains(sm, "gemma") || strings.Contains(sm, "learnlm") {
			continue
		}

		canGenerate := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				canGenerate = true
				break
			}
		}
		if canGenerate {
			candidates = append(candidates, id)
		}
	}

	// Rank and pick exactly the top 10 candidates for testing
	sort.Slice(candidates, func(i, j int) bool {
		return rankGeminiModel(candidates[i]) > rankGeminiModel(candidates[j])
	})
	if len(candidates) > 10 {
		candidates = candidates[:10]
	}

	// TIER 3 PERFORMANCE: Concurrently test health of all candidates simultaneously
	type checkResult struct {
		id     string
		passed bool
	}
	results := make(chan checkResult, len(candidates))
	var wg sync.WaitGroup

	for _, id := range candidates {
		wg.Add(1)
		go func(modelID string) {
			defer wg.Done()
			tCtx, cancel := context.WithTimeout(ctx, 5*time.Second) // Standard 5s timeout
			defer cancel()

			tp := &GeminiProvider{apiKey: p.apiKey, model: modelID, baseURL: p.baseURL}
			res, err := tp.Generate(tCtx, "Respond with ONLY the word Hello")
			passed := err == nil && strings.Contains(strings.ToLower(res), "hello")
			results <- checkResult{id: modelID, passed: passed}
		}(id)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var finalFlash, finalPro, finalMisc []string
	for res := range results {
		if !res.passed {
			continue
		}
		sm := strings.ToLower(res.id)
		if strings.Contains(sm, "flash") || strings.Contains(sm, "lite") {
			finalFlash = append(finalFlash, res.id)
		} else if strings.Contains(sm, "pro") {
			finalPro = append(finalPro, res.id)
		} else {
			finalMisc = append(finalMisc, res.id)
		}
	}

	// Priority Sorting
	sort.Slice(finalFlash, func(i, j int) bool { return rankGeminiModel(finalFlash[i]) > rankGeminiModel(finalFlash[j]) })
	sort.Slice(finalPro, func(i, j int) bool { return rankGeminiModel(finalPro[i]) > rankGeminiModel(finalPro[j]) })
	sort.Slice(finalMisc, func(i, j int) bool { return rankGeminiModel(finalMisc[i]) > rankGeminiModel(finalMisc[j]) })

	// TARGET: Exactly 3 models total (1 Pro + 2 Flash/Lite)
	output := []string{}
	
	// 1. Get top 1 Pro
	if len(finalPro) > 0 {
		output = append(output, finalPro[0])
		finalPro = finalPro[1:]
	}
	
	// 2. Get up to 2 Flash
	fLimit := 2
	if len(finalFlash) < fLimit { fLimit = len(finalFlash) }
	output = append(output, finalFlash[:fLimit]...)
	if len(finalFlash) > fLimit {
		finalFlash = finalFlash[fLimit:]
	} else {
		finalFlash = []string{}
	}

	// 3. FILLER: If we don't have exactly 3, fill from remaining healthy pools
	var pool []string
	pool = append(pool, finalFlash...)
	pool = append(pool, finalPro...)
	pool = append(pool, finalMisc...)
	sort.Slice(pool, func(i, j int) bool { return rankGeminiModel(pool[i]) > rankGeminiModel(pool[j]) })

	for len(output) < 3 && len(pool) > 0 {
		output = append(output, pool[0])
		pool = pool[1:]
	}

	return output, nil
}

func rankGeminiModel(m string) int {
	score := 0
	sm := strings.ToLower(m)

	// Tier 1: Prefer Flash-Lite and Flash for speed
	if strings.Contains(sm, "flash-lite") {
		score += 200
	} else if strings.Contains(sm, "flash") {
		score += 150
	} else if strings.Contains(sm, "pro") {
		score += 100
	}

	// Tier 2: PreferLatest over others
	if strings.Contains(sm, "latest") {
		score += 50
	}

	// Tier 3: STRONG penalty for Preview/Thinking/Ultra as requested
	if strings.Contains(sm, "preview") {
		score -= 1000
	}
	if strings.Contains(sm, "thinking") || strings.Contains(sm, "ultra") || strings.Contains(sm, "vision") {
		score -= 500
	}

	// Version weights
	if strings.Contains(sm, "3.1") {
		score += 40
	} else if strings.Contains(sm, "3.0") || (strings.Contains(sm, "3") && !strings.Contains(sm, "1.5")) {
		score += 35
	} else if strings.Contains(sm, "2.5") {
		score += 30
	} else if strings.Contains(sm, "2.0") || strings.Contains(sm, "2-") {
		score += 20
	} else if strings.Contains(sm, "1.5") {
		score += 10
	}

	return score
}
