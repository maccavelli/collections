package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIProvider_Generate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "chat/completions") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		
		auth := r.Header.Get("Authorization")
		if auth != "Bearer fake-openai-key" {
			t.Errorf("unexpected auth header: %s", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": "openai result",
					},
				},
			},
		})
	}))
	defer server.Close()

	p := NewOpenAI("fake-openai-key", "gpt-4")
	p.baseURL = server.URL

	got, err := p.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "openai result" {
		t.Errorf("expected openai result, got %s", got)
	}
}

func TestGeminiProvider_Generate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "fake-gemini-key") {
			t.Errorf("missing api key in query: %s", r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]string{
							{"text": "gemini result"},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	p, _ := NewGemini(context.Background(), "fake-gemini-key", "gemini-pro")
	p.baseURL = server.URL

	got, err := p.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "gemini result" {
		t.Errorf("expected gemini result, got %s", got)
	}
}

func TestAnthropicProvider_Generate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "fake-anthropic-key" {
			t.Errorf("unexpected api key: %s", r.Header.Get("x-api-key"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "anthropic result",
				},
			},
		})
	}))
	defer server.Close()

	p, _ := NewAnthropic("fake-anthropic-key", "claude-2")
	p.baseURL = server.URL

	got, err := p.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "anthropic result" {
		t.Errorf("expected anthropic result, got %s", got)
	}
}
