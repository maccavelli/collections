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
		mcp.WithString("format", mcp.Description("Output format: 'hybrid' (JSON metadata + markdown content), 'json' (pure structured data), or 'markdown' (pure narrative string)."), mcp.Enum("hybrid", "json", "markdown"), mcp.DefaultString("hybrid")),
	)
}

func (t *SearchTool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	maxResults := request.GetInt("max_results", 5)
	format := request.GetString("format", "hybrid")

	slog.Info("executing search", "type", t.Type, "query", query, "maxResults", maxResults, "format", format)
	results, err := t.SearchFunc(ctx, query, maxResults)
	if err != nil {
		slog.Error("search failed", "type", t.Type, "query", query, "error", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	response := models.SearchResponse{
		Type: t.Type,
		Metadata: &models.SearchMetadata{
			Query:      query,
			TotalCount: len(results),
			SearchType: t.Type,
		},
		Results: results,
	}

	switch format {
	case "markdown":
		return mcp.NewToolResultText(response.ToMarkdown()), nil
	case "json":
		res, err := mcp.NewToolResultJSON(response)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return res, nil
	default: // hybrid
		response.ResultsMD = response.ToMarkdown()
		// For hybrid, we keep original results but prioritize MD for ingestion
		res, err := mcp.NewToolResultJSON(response)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return res, nil
	}
}

// Register adds the search tools to the registry.
func Register(engine SearchEngine) {
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "web",
		SearchFunc: engine.WebSearch,
		Desc:       "Executes a high-concurrency web search to retrieve prioritized, high-relevance results. This tool leverages parallel provider querying to bypass SEO-saturated results and provide a clean set of web data. Use this for general information retrieval, verifying technical documentation, and cross-referencing facts during a research task.",
	})
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "news",
		SearchFunc: engine.NewsSearch,
		Desc:       "Conducts a targeted search across global news outlets for the most recent articles and reports. It is optimized for timeliness, ensuring that breaking developments and current events are prioritized over static content. Use this for monitoring live updates, gathering recent sentiment on a topic, or fact-checking news stories.",
	})
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "books",
		SearchFunc: engine.BookSearch,
		Desc:       "Performs a specialized search for literary and academic works, including authors, publishing dates, and ISBN-level metadata. This tool is essential for academic research and verifying citations in formal documentation. Use this to locate primary sources, research book summaries, or build comprehensive bibliographies.",
	})
}
