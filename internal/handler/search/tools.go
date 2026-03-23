package search

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-duckduckgo/internal/models"
	"mcp-server-duckduckgo/internal/registry"
)

// SearchEngine defines the interface for engine searches.
type SearchEngine interface {
	WebSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error)
	NewsSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error)
	BookSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error)
}

// SearchTool implements Tool for various search types.
type SearchTool struct {
	Engine     SearchEngine
	Type       string
	SearchFunc func(context.Context, string, int) ([]models.SearchResult, error)
	Desc       string
}

func (t *SearchTool) Metadata() mcp.Tool {
	name := fmt.Sprintf("ddg_search_%s", t.Type)
	return mcp.NewTool(name,
		mcp.WithDescription(t.Desc),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5). Low counts are faster and more token-efficient."), mcp.DefaultNumber(5)),
	)
}

func (t *SearchTool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	maxResults := request.GetInt("max_results", 5)

	slog.Info("executing search", "type", t.Type, "query", query, "maxResults", maxResults)
	results, err := t.SearchFunc(ctx, query, maxResults)
	if err != nil {
		slog.Error("search failed", "type", t.Type, "query", query, "error", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(models.SearchResponse{
		Type:    t.Type,
		Results: results,
	})
}

// Register adds the search tools to the registry.
func Register(engine SearchEngine) {
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "web",
		SearchFunc: engine.WebSearch,
		Desc:       "Perform a high-quality web search using DuckDuckGo. Use lower max_results for efficiency.",
	})
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "news",
		SearchFunc: engine.NewsSearch,
		Desc:       "Perform a news-specific search using DuckDuckGo. Use lower max_results for efficiency.",
	})
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "books",
		SearchFunc: engine.BookSearch,
		Desc:       "Search for books using DuckDuckGo. Use lower max_results for efficiency.",
	})
}
