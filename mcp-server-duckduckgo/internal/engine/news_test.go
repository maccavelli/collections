package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewsSearch(t *testing.T) {
	vqdResp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}

	t.Run("success_with_various_dates", func(t *testing.T) {
		jsonResp := `{
			"results": [
				{"title": "T1", "url": "U1", "excerpt": "S1", "source": "Src1", "date": "2024-01-01", "image": "I1"},
				{"title": "T2", "url": "U2", "excerpt": "S2", "source": "Src2", "date": 1704067200, "image": "I2"}
			]
		}`
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 {
				return vqdResp, nil
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(jsonResp))}, nil
		})
		e := NewSearchEngine()
		e.Client = client
		results, err := e.NewsSearch(context.Background(), "test", 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("vqd_error", func(t *testing.T) {
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`no vqd`))}, nil
		})
		e := NewSearchEngine()
		e.Client = client
		_, err := e.NewsSearch(context.Background(), "test", 1)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("decode_error", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`invalid json`))}, nil
		})
		e := NewSearchEngine()
		e.Client = client
		_, err := e.NewsSearch(context.Background(), "test", 1)
		if err == nil || !strings.Contains(err.Error(), "failed to decode") {
			t.Errorf("expected decode error, got %v", err)
		}
	})

	t.Run("news_http_error", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil
			}
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		})
		e := NewSearchEngine()
		e.Client = client
		_, err := e.NewsSearch(context.Background(), "test", 1)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("news_transport_error", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil
			}
			return nil, fmt.Errorf("transport failure")
		})
		e := NewSearchEngine()
		e.Client = client
		_, err := e.NewsSearch(context.Background(), "test", 1)
		if err == nil {
			t.Error("expected error")
		}
	})
}
