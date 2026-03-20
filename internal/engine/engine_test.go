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

// mockTransport implements http.RoundTripper for mocking.
type mockTransport struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

func newMockClient(roundTrip func(*http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{Transport: &mockTransport{roundTrip}}
}

func TestGetVQD(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`vqd='123456789'`)),
			}, nil
		})
		e := &SearchEngine{Client: client}
		vqd, err := e.getVQD(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vqd != "123456789" {
			t.Errorf("expected 123456789, got %s", vqd)
		}
	})

	t.Run("failure_not_found", func(t *testing.T) {
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`no vqd here`)),
			}, nil
		})
		e := &SearchEngine{Client: client}
		_, err := e.getVQD(context.Background(), "test")
		if err == nil || !strings.Contains(err.Error(), "could not extract vqd") {
			t.Errorf("expected extraction error, got %v", err)
		}
	})

	t.Run("network_error", func(t *testing.T) {
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("network down")
		})
		e := &SearchEngine{Client: client}
		_, err := e.getVQD(context.Background(), "test")
		if err == nil || !strings.Contains(err.Error(), "network down") {
			t.Errorf("expected network error, got %v", err)
		}
	})
}

func TestWebSearch_Coverage(t *testing.T) {
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
		e := &SearchEngine{Client: client}
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
		e := &SearchEngine{Client: client}
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
		e := &SearchEngine{Client: client}
		_, err := e.WebSearch(context.Background(), "test", 1)
		if err == nil {
			t.Error("expected parsing error, got nil")
		}
	})
}


func TestNewsSearch_Coverage(t *testing.T) {
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
			if count == 1 { return vqdResp, nil }
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(jsonResp))}, nil
		})
		e := &SearchEngine{Client: client}
		results, err := e.NewsSearch(context.Background(), "test", 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("decode_error", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 { return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil }
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`invalid json`))}, nil
		})
		e := &SearchEngine{Client: client}
		_, err := e.NewsSearch(context.Background(), "test", 1)
		if err == nil || !strings.Contains(err.Error(), "failed to decode") {
			t.Errorf("expected decode error, got %v", err)
		}
	})
	t.Run("vqd_error", func(t *testing.T) {
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`no vqd`))}, nil
		})
		e := &SearchEngine{Client: client}
		_, err := e.NewsSearch(context.Background(), "test", 1)
		if err == nil { t.Error("expected error") }
	})

	t.Run("news_http_error", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 { return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil }
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		})
		e := &SearchEngine{Client: client}
		_, err := e.NewsSearch(context.Background(), "test", 1)
		if err == nil { t.Error("expected error") }
	})

	t.Run("news_transport_error", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 { return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil }
			return nil, fmt.Errorf("transport failure")
		})
		e := &SearchEngine{Client: client}
		_, err := e.NewsSearch(context.Background(), "test", 1)
		if err == nil { t.Error("expected error") }
	})
}



func TestImageAndVideoSearch_Coverage(t *testing.T) {
	t.Run("images_success", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 { return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil }
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"results": [{"title": "I1", "image": "Img1"}]}`)),
			}, nil
		})
		e := &SearchEngine{Client: client}
		results, err := e.ImageSearch(context.Background(), "test", 1)
		if err != nil || len(results) != 1 {
			t.Errorf("image search failed: %v, len=%d", err, len(results))
		}
	})

	t.Run("videos_success", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 { return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil }
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"results": [{"title": "V1", "description": "D1"}]}`)),
			}, nil
		})
		e := &SearchEngine{Client: client}
		results, err := e.VideoSearch(context.Background(), "test", 1)
		if err != nil || len(results) != 1 {
			t.Errorf("video search failed: %v, len=%d", err, len(results))
		}
	})

	t.Run("image_vqd_error", func(t *testing.T) {
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`no vqd`))}, nil
		})
		e := &SearchEngine{Client: client}
		_, err := e.ImageSearch(context.Background(), "test", 1)
		if err == nil { t.Error("expected error") }
	})

	t.Run("video_vqd_error", func(t *testing.T) {
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`no vqd`))}, nil
		})
		e := &SearchEngine{Client: client}
		_, err := e.VideoSearch(context.Background(), "test", 1)
		if err == nil { t.Error("expected error") }
	})

	t.Run("image_decode_error", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 { return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil }
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`invalid json`))}, nil
		})
		e := &SearchEngine{Client: client}
		_, err := e.ImageSearch(context.Background(), "test", 1)
		if err == nil { t.Error("expected error") }
	})

	t.Run("video_decode_error", func(t *testing.T) {
		count := 0
		client := newMockClient(func(req *http.Request) (*http.Response, error) {
			count++
			if count == 1 { return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`vqd='123'`))}, nil }
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`invalid json`))}, nil
		})
		e := &SearchEngine{Client: client}
		_, err := e.VideoSearch(context.Background(), "test", 1)
		if err == nil { t.Error("expected error") }
	})
}



type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) { return 0, fmt.Errorf("read error") }

