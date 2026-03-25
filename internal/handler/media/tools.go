package media

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

func (t *MediaTool) Metadata() mcp.Tool {
	name := fmt.Sprintf("ddg_search_%s", t.Type)
	return mcp.NewTool(name,
		mcp.WithDescription(t.Desc),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5). Low counts are faster and more token-efficient."), mcp.DefaultNumber(5)),
	)
}

func (t *MediaTool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	maxResults := request.GetInt("max_results", 5)

	slog.Info("executing media search", "type", t.Type, "query", query, "maxResults", maxResults)
	results, err := t.SearchFunc(ctx, query, maxResults)
	if err != nil {
		slog.Error("media search failed", "type", t.Type, "query", query, "error", err)
		return mcp.NewToolResultError(err.Error()), nil
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
			Query:      query,
			TotalCount: len(results),
			SearchType: t.Type,
		},
		Results: mediaResults,
	}

	res, err := mcp.NewToolResultJSON(response)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return res, nil
}

// Register adds the media tools to the registry.
func Register(engine SearchEngine) {
	registry.Global.Register(&MediaTool{
		Engine:     engine,
		Type:       "images",
		SearchFunc: engine.ImageSearch,
		Desc:       "Retrieves visual assets and media metadata from the web. This tool is designed for asset discovery, providing high-quality image URLs and source information while maintaining a low-latency response profile. Use this for UI/UX design inspiration, finding technical diagrams, or sourcing creative assets for projects.",
	})
	registry.Global.Register(&MediaTool{
		Engine:     engine,
		Type:       "videos",
		SearchFunc: engine.VideoSearch,
		Desc:       "Searches for video content, including tutorials, demonstrations, and multimedia reports. It extracts key metadata such as durations and publishers to help filter for the most relevant content quickly. Use this for locating \"how-to\" guides, technical walkthroughs, or verifying video-based news sources.",
	})
}
