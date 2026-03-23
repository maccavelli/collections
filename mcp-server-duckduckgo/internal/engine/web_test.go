package engine

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWebSearch(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		html := `
			<div class="web-result">
				<a class="result__a" href="http://example.com">Title</a>
				<div class="result__title">Title</div>
				<div class="result__snippet">Snippet</div>
			</div>`
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(html)),
			}, nil
		})
		e := NewSearchEngine()
		e.Client = client
		results, err := e.WebSearch(context.Background(), "test", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 || results[0].URL != "http://example.com" {
			t.Errorf("unexpected results: %+v", results)
		}
	})

	t.Run("http_error", func(t *testing.T) {
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		})
		e := NewSearchEngine()
		e.Client = client
		_, err := e.WebSearch(context.Background(), "test", 1)
		if err == nil || !strings.Contains(err.Error(), "status code: 500") {
			t.Errorf("expected 500 status error, got %v", err)
		}
	})

	t.Run("parsing_error", func(t *testing.T) {
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(&errorReader{}),
			}, nil
		})
		e := NewSearchEngine()
		e.Client = client
		_, err := e.WebSearch(context.Background(), "test", 1)
		if err == nil {
			t.Error("expected parsing error, got nil")
		}
	})
}
