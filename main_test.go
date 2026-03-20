package main

import (
	"context"
	"fmt"
	"os"
	"testing"


	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-duckduckgo/internal/engine"
	"mcp-server-duckduckgo/internal/models"
)

func TestMain_Coverage(t *testing.T) {
	// We can't easily test main() because of ServeStdio blocking and os.Exit.
	// But we can test run() with --version which covers the main logic.
	t.Run("main_version", func(t *testing.T) {
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()
		os.Args = []string{"cmd", "--version"}
		main()
	})
}


func TestRun(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		if err := run(context.Background(), []string{"cmd", "--version"}); err != nil {
			t.Errorf("run --version failed: %v", err)
		}
	})

	t.Run("flag_error", func(t *testing.T) {
		if err := run(context.Background(), []string{"cmd", "--invalid"}); err == nil {
			t.Error("expected error for invalid flag")
		}
	})
}

func TestNewServer(t *testing.T) {
	s := newServer(engine.NewSearchEngine())
	if s == nil {
		t.Fatal("expected server to be created")
	}
	// Check if tools are added
	tools := s.ListTools()
	if len(tools) != 5 {
		t.Errorf("expected 5 tools, got %d", len(tools))
	}
}

func TestMakeSearchHandler(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockSearch := func(ctx context.Context, query string, max int) ([]models.SearchResult, error) {
			return []models.SearchResult{{Title: "Result"}}, nil
		}
		handler := makeSearchHandler(mockSearch, "test")
		
		// Manually construct the request as NewCallToolRequest is undefined
		req := mcp.CallToolRequest{}
		req.Params.Name = "ddg_search_web"
		req.Params.Arguments = map[string]interface{}{"query": "test"}
		
		res, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IsError {
			t.Fatal("expected no error in result")
		}
	})

	t.Run("missing_query", func(t *testing.T) {
		handler := makeSearchHandler(nil, "test")
		req := mcp.CallToolRequest{}
		req.Params.Name = "ddg_search_web"
		req.Params.Arguments = map[string]interface{}{}
		
		res, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if !res.IsError {
			t.Error("expected error result due to missing query")
		}
	})

	t.Run("engine_error", func(t *testing.T) {
		mockSearch := func(ctx context.Context, query string, max int) ([]models.SearchResult, error) {
			return nil, fmt.Errorf("engine failure")
		}
		handler := makeSearchHandler(mockSearch, "test")
		req := mcp.CallToolRequest{}
		req.Params.Name = "ddg_search_web"
		req.Params.Arguments = map[string]interface{}{"query": "test"}
		
		res, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected handler error: %v", err)
		}
		if !res.IsError {
			t.Error("expected error result due to engine failure")
		}
	})
}
