package media

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-duckduckgo/internal/models"
)

type mockMediaEngine struct{}

func (m *mockMediaEngine) ImageSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	return []models.SearchResult{{Title: "Image Result"}}, nil
}
func (m *mockMediaEngine) VideoSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error) {
	return []models.SearchResult{{Title: "Video Result"}}, nil
}

func TestMediaTool_Handle(t *testing.T) {
	engine := &mockMediaEngine{}
	tool := &MediaTool{
		Engine:     engine,
		Type:       "images",
		SearchFunc: engine.ImageSearch,
		Desc:       "Test description",
	}

	ctx := context.Background()
	input := MediaInput{
		Query:      "test image",
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
	if tool.Name() != "ddg_search_images" {
		t.Errorf("expected ddg_search_images, got %s", tool.Name())
	}
}
