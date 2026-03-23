package engine

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestVideoSearch(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"results": [{"title": "V1", "description": "D1"}]}`)),
			}, nil
		})
		e := NewSearchEngine(); e.Client = client
		results, err := e.VideoSearch(context.Background(), "test", 1)
		if err != nil || len(results) != 1 {
			t.Errorf("video search failed: %v, len=%d", err, len(results))
		}
	})

	t.Run("vqd_error", func(t *testing.T) {
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`no vqd`))}, nil
		})
		e := NewSearchEngine(); e.Client = client
		_, err := e.VideoSearch(context.Background(), "test", 1)
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
		e := NewSearchEngine(); e.Client = client
		_, err := e.VideoSearch(context.Background(), "test", 1)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("http_error", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil
			}
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(`error`))}, nil
		})
		e := NewSearchEngine(); e.Client = client
		_, err := e.VideoSearch(context.Background(), "test", 1)
		if err == nil || !strings.Contains(err.Error(), "status code: 500") {
			t.Errorf("expected 500 error, got %v", err)
		}
	})
}
