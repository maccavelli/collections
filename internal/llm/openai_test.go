package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAI_Name(t *testing.T) {
	p := NewOpenAI("key", "model")
	if p.Name() != "openai" {
		t.Error("expected openai")
	}
}

func TestOpenAI_Generate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "Bearer error" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if auth == "Bearer badjson" {
			w.Write([]byte("{bad"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices": [{"message": {"content": "mock response"}}]}`))
	}))
	defer ts.Close()

	ctx := context.Background()

	p := &OpenAIProvider{apiKey: "error", baseURL: ts.URL}
	_, err := p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected error")
	}

	p = &OpenAIProvider{apiKey: "badjson", baseURL: ts.URL}
	_, err = p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected error")
	}

	p = &OpenAIProvider{apiKey: "success", baseURL: ts.URL}
	res, err := p.Generate(ctx, "hello")
	if err != nil || res != "mock response" {
		t.Errorf("expected success, got err=%v, res=%s", err, res)
	}
}

func TestOpenAI_DiscoverModels(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices": [{"message": {"content": "Hello"}}]}`))
	}))
	defer ts.Close()

	ctx := context.Background()

	p := &OpenAIProvider{apiKey: "success", baseURL: ts.URL}
	models, err := p.DiscoverModels(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 3 {
		t.Errorf("expected all fail-safes to return hello due to mock, got %d", len(models))
	}
}

func TestOpenAI_Errors(t *testing.T) {
	ctx := context.Background()

	// NewRequest error
	p := &OpenAIProvider{apiKey: "success", baseURL: string([]byte{0x7f})}
	_, err := p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected Request creation error")
	}

	// Do error
	p = &OpenAIProvider{apiKey: "success", baseURL: "http://localhost:0"}
	_, err = p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected network error")
	}

	// Empty content
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices": []}`))
	}))
	defer ts.Close()
	p = &OpenAIProvider{apiKey: "success", baseURL: ts.URL}
	_, err = p.Generate(ctx, "hello")
	if err == nil {
		t.Error("expected empty choices error")
	}
}
