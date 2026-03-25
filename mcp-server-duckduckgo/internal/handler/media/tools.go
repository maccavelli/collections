package media

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-duckduckgo/internal/models"
	"mcp-server-duckduckgo/internal/registry"
)

// SearchEngine defines the interface for engine searches.
type SearchEngine interface {
	ImageSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error)
	VideoSearch(ctx context.Context, query string, maxResults int) ([]models.SearchResult, error)
}

// MediaTool implements Tool for various media search types.
type MediaTool struct {
	Engine     SearchEngine
	Type       string
	SearchFunc func(context.Context, string, int) ([]models.SearchResult, error)
	Desc       string
}

func (t *MediaTool) Name() string {
	return fmt.Sprintf("ddg_search_%s", t.Type)
}

type MediaInput struct {
	Query      string `json:"query" jsonschema:"The search keywords"`
	MaxResults int    `json:"max_results" jsonschema:"Maximum results to return (default 5). Low counts are faster and more token-efficient."`
}

func (t *MediaTool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: t.Desc,
	}, t.Handle)
}

func (t *MediaTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input MediaInput) (*mcp.CallToolResult, any, error) {
	if input.MaxResults <= 0 {
		input.MaxResults = 5
	}

	results, err := t.SearchFunc(ctx, input.Query, input.MaxResults)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	mediaResults := make([]models.MediaResult20, 0, len(results))
	for _, r := range results {
		mediaURL := r.ImageURL
		if mediaURL == "" && t.Type == "videos" {
			mediaURL = r.URL
		}
		mediaResults = append(mediaResults, models.MediaResult20{
			Title:        r.Title,
			PageURL:      r.URL,
			MediaURL:     mediaURL,
			ThumbnailURL: r.Thumbnail,
			Duration:     r.Duration,
			Publisher:    r.Publisher,
			Source:       r.Source,
		})
	}

	response := models.SearchResponse20{
		Version: "2.0",
		Metadata: &models.SearchMetadata{
			Query:      input.Query,
			TotalCount: len(results),
			SearchType: t.Type,
		},
		Results: mediaResults,
	}

	return &mcp.CallToolResult{}, response, nil
}

// Register adds the media tools to the registry.
func Register(engine SearchEngine) {
	registry.Global.Register(&MediaTool{
		Engine:     engine,
		Type:       "images",
		SearchFunc: engine.ImageSearch,
		Desc:       "VISUAL DISCOVERY: High-priority asset retrieval. Call this when the task requires UI/UX inspiration, technical diagrams, or creative assets. Cascades to ddg_search_videos for demonstrations.",
	})
	registry.Global.Register(&MediaTool{
		Engine:     engine,
		Type:       "videos",
		SearchFunc: engine.VideoSearch,
		Desc:       "DEMONSTRATION RETRIEVAL: Targeted search for tutorials, walkthroughs, and multimedia reports. Call this when a text-based explanation is insufficient. Cascades to ddg_search_web for written documentation.",
	})
}
