package search

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-duckduckgo/internal/models"
	"mcp-server-duckduckgo/internal/registry"
	"mcp-server-duckduckgo/internal/util"
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

func (t *SearchTool) Name() string {
	return fmt.Sprintf("search_%s", t.Type)
}

type SearchInput struct {
	Query      string `json:"query" jsonschema:"The primary search query string to execute"`
	MaxResults int    `json:"max_results" jsonschema:"The maximum number of search results to return. Default is 5."`
	Format     string `json:"format" jsonschema:"Output format: 'hybrid', 'json', or 'markdown'.,enum=hybrid,enum=json,enum=markdown"`
}

func (t *SearchTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: t.Desc,
	}, t.Handle)
}

func (t *SearchTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, any, error) {
	slog.Info("[BACKPLANE] [SearchTool] executing search action", "type", t.Type, "query", input.Query, "format", input.Format)
	if input.MaxResults <= 0 {
		input.MaxResults = 5
	}
	if input.Format == "" {
		input.Format = os.Getenv("DDG_DEFAULT_FORMAT")
		if input.Format == "" {
			input.Format = "json"
		}
	}

	results, err := t.SearchFunc(ctx, input.Query, input.MaxResults)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	response := models.SearchResponse{
		Summary: fmt.Sprintf("Found %d %s results for '%s'", len(results), t.Type, input.Query),
	}
	response.Data.Type = t.Type
	response.Data.Metadata = &models.SearchMetadata{
		Query:      input.Query,
		TotalCount: len(results),
		SearchType: t.Type,
	}
	response.Data.Results = results

	switch input.Format {
	case "markdown":
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: response.ToMarkdown()}},
		}, nil, nil
	case "hybrid":
		response.ResultsMD = response.ToMarkdown()
		return &mcp.CallToolResult{}, response, nil
	default: // json
		return &mcp.CallToolResult{}, response, nil
	}
}

// Register adds the search tools to the registry.
func Register(engine SearchEngine) {
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "web",
		SearchFunc: engine.WebSearch,
		Desc:       "[DIRECTIVE: General Internet Discovery] Comprehensive retrieval utilizing the DuckDuckGo index to extract current, real-time web results and engine queries. Keywords: web, internet, websites, online, general-search, urls, text, browse",
	})
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "news",
		SearchFunc: engine.NewsSearch,
		Desc:       "[DIRECTIVE: Live Events and Journalism] Preferred choice for breaking developments, real-time updates, and current timeline information. Keywords: news, press, breaking, events, articles, temporal, journalism, headlines",
	})
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "books",
		SearchFunc: engine.BookSearch,
		Desc:       "[DIRECTIVE: Academic and Scholarship Lookup] Specialized retrieval of authoritative literature, academic citations, and literary metadata. Keywords: books, academic, scholars, publications, isbn, citations, authors, reading",
	})
}
