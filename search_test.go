package main

import (
	"testing"
)

func TestWebSearch(t *testing.T) {
	engine := NewSearchEngine()
	results, err := engine.WebSearch("Golang", 1)
	if err != nil {
		t.Fatalf("WebSearch failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("WebSearch returned no results")
	}
}

func TestNewsSearch(t *testing.T) {
	engine := NewSearchEngine()
	results, err := engine.NewsSearch("Golang", 1)
	if err != nil {
		t.Fatalf("NewsSearch failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("NewsSearch returned no results")
	}
}

func TestImageSearch(t *testing.T) {
	engine := NewSearchEngine()
	results, err := engine.ImageSearch("Golang", 1)
	if err != nil {
		t.Fatalf("ImageSearch failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("ImageSearch returned no results")
	}
}

func TestVideoSearch(t *testing.T) {
	engine := NewSearchEngine()
	results, err := engine.VideoSearch("Golang", 1)
	if err != nil {
		t.Fatalf("VideoSearch failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("VideoSearch returned no results")
	}
}

func TestBookSearch(t *testing.T) {
	engine := NewSearchEngine()
	results, err := engine.BookSearch("Golang", 1)
	if err != nil {
		t.Fatalf("BookSearch failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("BookSearch returned no results")
	}
}
