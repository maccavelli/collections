package search

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

func (t *SearchTool) Name() string {
	return fmt.Sprintf("ddg_search_%s", t.Type)
}

type SearchInput struct {
	Query      string `json:"query" jsonschema:"The search keywords"`
	MaxResults int    `json:"max_results" jsonschema:"Maximum results to return (default 5). Low counts are faster and more token-efficient."`
	Format     string `json:"format" jsonschema:"Output format: 'hybrid' (JSON metadata + markdown content), 'json' (pure structured data), or 'markdown' (pure narrative string).,enum=hybrid,enum=json,enum=markdown"`
}

func (t *SearchTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: t.Desc,
	}, t.Handle)
}

func (t *SearchTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, any, error) {
	if input.MaxResults <= 0 {
		input.MaxResults = 5
	}
	if input.Format == "" {
		input.Format = "hybrid"
	}

	results, err := t.SearchFunc(ctx, input.Query, input.MaxResults)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	response := models.SearchResponse{
		Type: t.Type,
		Metadata: &models.SearchMetadata{
			Query:      input.Query,
			TotalCount: len(results),
			SearchType: t.Type,
		},
		Results: results,
	}

	switch input.Format {
	case "markdown":
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: response.ToMarkdown()}},
		}, nil, nil
	case "json":
		return &mcp.CallToolResult{}, response, nil
	default: // hybrid
		response.ResultsMD = response.ToMarkdown()
		return &mcp.CallToolResult{}, response, nil
	}
}

// Register adds the search tools to the registry.
func Register(engine SearchEngine) {
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "web",
		SearchFunc: engine.WebSearch,
		Desc:       "PRIMARY RESEARCH MANDATE: High-concurrency entry point for general intelligence. Call this FIRST for any information retrieval task. Cascades to ddg_search_news for current events or ddg_search_images for visual assets.",
	})
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "news",
		SearchFunc: engine.NewsSearch,
		Desc:       "TIMELINESS MANDATE: Targeted search for breaking developments and live updates. Call this if ddg_search_web returns stale data or when investigating specific current events. Cascades to ddg_search_web for historical context.",
	})
	registry.Global.Register(&SearchTool{
		Engine:     engine,
		Type:       "books",
		SearchFunc: engine.BookSearch,
		Desc:       "ACADEMIC AUDIT: Specialized retrieval for authoritative sources, citations, and literary metadata. Call this to verify facts found in ddg_search_web or when performing deep scholarly research. Cascades to ddg_search_web for broader context.",
	})
}
