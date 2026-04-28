package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropic_Name(t *testing.T) {
	p, _ := NewAnthropic("key", "model")
	if p.Name() != "anthropic" {
		t.Error("expected anthropic")
	}
}

func TestAnthropic_Generate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("x-api-key")
		if auth == "error" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if auth == "badjson" {
			w.Write([]byte("{bad"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content": [{"text": "mock response"}]}`))
	}))
	defer ts.Close()

	ctx := context.Background()

	p := &AnthropicProvider{apiKey: "error", baseURL: ts.URL}
	_, err := p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected error")
	}

	p = &AnthropicProvider{apiKey: "badjson", baseURL: ts.URL}
	_, err = p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected error")
	}

	p = &AnthropicProvider{apiKey: "success", baseURL: ts.URL}
	res, err := p.Generate(ctx, "hello")
	if err != nil || res != "mock response" {
		t.Errorf("expected success, got err=%v, res=%s", err, res)
	}
}

func TestAnthropic_DiscoverModels(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content": [{"text": "Hello"}]}`))
	}))
	defer ts.Close()

	ctx := context.Background()

	p := &AnthropicProvider{apiKey: "success", baseURL: ts.URL}
	models, err := p.DiscoverModels(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected all models, got %d", len(models))
	}
}

func TestNewAnthropic_EmptyKey(t *testing.T) {
	_, err := NewAnthropic("", "model")
	if err == nil {
		t.Error("expected err empty key")
	}
}

func TestAnthropic_Errors(t *testing.T) {
	ctx := context.Background()

	// NewRequest error
	p := &AnthropicProvider{apiKey: "success", baseURL: string([]byte{0x7f})}
	_, err := p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected Request creation error")
	}

	// Do error
	p = &AnthropicProvider{apiKey: "success", baseURL: "http://localhost:0"}
	_, err = p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected network error")
	}

	// Empty content
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content": []}`))
	}))
	defer ts.Close()
	p = &AnthropicProvider{apiKey: "success", baseURL: ts.URL}
	_, err = p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected empty content error")
	}
}
