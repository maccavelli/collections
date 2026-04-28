package search

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-duckduckgo/internal/models"
)

type mockSearchEngine struct{}

func (m *mockSearchEngine) WebSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	return []models.SearchResult{{Title: "Web Result"}}, nil
}
func (m *mockSearchEngine) NewsSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	return []models.SearchResult{{Title: "News Result"}}, nil
}
func (m *mockSearchEngine) BookSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	return []models.SearchResult{{Title: "Book Result"}}, nil
}

func TestSearchTool_Handle(t *testing.T) {
	engine := &mockSearchEngine{}
	tool := &SearchTool{
		Engine:     engine,
		Type:       "web",
		SearchFunc: engine.WebSearch,
		Desc:       "Test description",
	}

	ctx := context.Background()
	input := SearchInput{
		Query:      "test query",
		MaxResults: 1,
	}


	// Test Handle
	_, resp, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Errorf("Handle failed: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}

	// Verify Name
	if tool.Name() != "search_web" {
		t.Errorf("expected search_web, got %s", tool.Name())
	}
}

func TestRegister(t *testing.T) {
	eng := &mockSearchEngine{}
	Register(eng)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, &mcp.ServerOptions{})
	tool := &SearchTool{Engine: eng, Type: "web"}
	tool.Register(srv)
}
