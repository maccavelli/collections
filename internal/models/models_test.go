package models

import (
	"encoding/json"
	"testing"
)

func TestSearchResultJSON(t *testing.T) {
	res := SearchResult{
		Title: "Test",
		URL:   "http://example.com",
	}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var res2 SearchResult
	if err := json.Unmarshal(data, &res2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if res2.Title != res.Title || res2.URL != res.URL {
		t.Errorf("expected %+v, got %+v", res, res2)
	}
}

func TestSearchResponseJSON(t *testing.T) {
	resp := SearchResponse{
		Type: "web",
		Results: []SearchResult{
			{Title: "R1", URL: "U1"},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var resp2 SearchResponse
	if err := json.Unmarshal(data, &resp2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp2.Type != resp.Type || len(resp2.Results) != 1 {
		t.Errorf("expected %+v, got %+v", resp, resp2)
	}
}
