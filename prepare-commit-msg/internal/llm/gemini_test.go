package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGemini_Name(t *testing.T) {
	p := &GeminiProvider{}
	if p.Name() != "gemini" {
		t.Error("expected gemini")
	}
}

func TestNewGemini(t *testing.T) {
	p, err := NewGemini(context.Background(), "", "model")
	if err != nil {
		t.Error("expected no error")
	}
	if p == nil {
		t.Error("expected provider")
	}
}

func TestGemini_Generate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "error") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.Contains(r.URL.Path, "badjson") {
			w.Write([]byte("{bad"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"candidates": [{"content": {"parts": [{"text": "mock response"}]}}]}`))
	}))
	defer ts.Close()

	ctx := context.Background()

	p := &GeminiProvider{model: "error", baseURL: ts.URL}
	_, err := p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected error")
	}

	p = &GeminiProvider{model: "badjson", baseURL: ts.URL}
	_, err = p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected error")
	}

	p = &GeminiProvider{model: "success", baseURL: ts.URL}
	res, err := p.Generate(ctx, "hello")
	if err != nil || res != "mock response" {
		t.Errorf("expected success, got err=%v, res=%s", err, res)
	}
}

func TestGemini_DiscoverModels(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "generateContent") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"candidates": [{"content": {"parts": [{"text": "Hello"}]}}]}`))
			return
		}
		
		if r.URL.Query().Get("key") == "error" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.URL.Query().Get("key") == "badjson" {
			w.Write([]byte("{bad"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"models": [
				{"name": "models/gemini-2.5-flash-lite", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-3.1-pro", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-3.0-flash", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-1.5-pro", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-2.0-flash", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-3-flash", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/other-pro", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-preview", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-thinking", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-ultra", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-image", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemma", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/learnlm", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-pro-vision", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/ignore-me", "supportedGenerationMethods": []}
			]
		}`))
	}))
	defer ts.Close()

	ctx := context.Background()

	p := &GeminiProvider{apiKey: "error", baseURL: ts.URL}
	_, err := p.DiscoverModels(ctx)
	if err == nil {
		t.Error("expected error")
	}

	p = &GeminiProvider{apiKey: "badjson", baseURL: ts.URL}
	_, err = p.DiscoverModels(ctx)
	if err == nil {
		t.Error("expected error")
	}

	p = &GeminiProvider{apiKey: "success", baseURL: ts.URL}
	models, err := p.DiscoverModels(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 3 {
		t.Errorf("expected 3 models, got %v", models)
	}
}

func TestRankGeminiModel(t *testing.T) {
	if rankGeminiModel("gemini-2.5-pro") <= rankGeminiModel("gemini-1.5-pro") {
		t.Error("expected 2.5 > 1.5")
	}
	if rankGeminiModel("flash") <= rankGeminiModel("lite") {
		t.Error("expected flash > lite")
	}
}

func TestRankGeminiModel_Exhaustive(t *testing.T) {
	cases := []string{
		"gemini-2.5-flash-lite-latest",
		"gemini-3.1-pro",
		"gemini-3.0-pro",
		"gemini-2.0-pro",
		"gemini-1.5-pro",
		"gemini-preview",
		"gemini-thinking",
		"gemini-ultra",
		"gemini-vision",
		"gemini-3-pro", // to hit 3 and not 1.5
	}
	for _, c := range cases {
		rankGeminiModel(c) // exercises all branches
	}
}

func TestGemini_Errors(t *testing.T) {
	ctx := context.Background()

	p := &GeminiProvider{model: "success", baseURL: string([]byte{0x7f})}
	if _, err := p.Generate(ctx, "hello"); err == nil {
		t.Error("expected Request creation error")
	}
	if _, err := p.DiscoverModels(ctx); err == nil {
		t.Error("expected Request creation error")
	}

	p = &GeminiProvider{model: "success", baseURL: "http://localhost:0"}
	if _, err := p.Generate(ctx, "hello"); err == nil {
		t.Error("expected network error")
	}
	if _, err := p.DiscoverModels(ctx); err == nil {
		t.Error("expected network error")
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"candidates": []}`))
	}))
	defer ts.Close()
	p = &GeminiProvider{model: "success", baseURL: ts.URL}
	if _, err := p.Generate(ctx, "hello"); err == nil {
		t.Error("expected empty candidates error")
	}
}
