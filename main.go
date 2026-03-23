package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"mcp-server-duckduckgo/internal/engine"
	"mcp-server-duckduckgo/internal/models"
)

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("mcp-server-duckduckgo version %s\n", Version)
		os.Exit(0)
	}

	setupLogging()
	slog.Info("starting mcp-server-duckduckgo", "version", Version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("server fatal error", "error", err)
		os.Exit(1)
	}
}

func setupLogging() {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))
}

func run(ctx context.Context) error {
	s := newServer(engine.NewSearchEngine())
	
	go func() {
		<-ctx.Done()
		slog.Info("shutdown signal received; stopping server")
	}()

	return server.ServeStdio(s)
}

func newServer(searchEngine *engine.SearchEngine) *server.MCPServer {
	s := server.NewMCPServer(
		"DuckDuckGo Search",
		Version,
		server.WithLogging(),
	)

	// Register tools
	s.AddTool(mcp.NewTool("ddg_search_web",
		mcp.WithDescription("Perform a high-quality web search using DuckDuckGo. Use lower max_results for efficiency."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5). Low counts are faster and more token-efficient."), mcp.DefaultNumber(5)),
	), makeSearchHandler(searchEngine.WebSearch, "web"))

	s.AddTool(mcp.NewTool("ddg_search_news",
		mcp.WithDescription("Perform a news-specific search using DuckDuckGo. Use lower max_results for efficiency."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5). Low counts are faster and more token-efficient."), mcp.DefaultNumber(5)),
	), makeSearchHandler(searchEngine.NewsSearch, "news"))

	s.AddTool(mcp.NewTool("ddg_search_images",
		mcp.WithDescription("Search for images using DuckDuckGo. Use lower max_results for efficiency."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5). Low counts are faster and more token-efficient."), mcp.DefaultNumber(5)),
	), makeSearchHandler(searchEngine.ImageSearch, "image"))

	s.AddTool(mcp.NewTool("ddg_search_videos",
		mcp.WithDescription("Search for videos using DuckDuckGo. Use lower max_results for efficiency."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5). Low counts are faster and more token-efficient."), mcp.DefaultNumber(5)),
	), makeSearchHandler(searchEngine.VideoSearch, "video"))

	s.AddTool(mcp.NewTool("ddg_search_books",
		mcp.WithDescription("Search for books using DuckDuckGo. Use lower max_results for efficiency."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5). Low counts are faster and more token-efficient."), mcp.DefaultNumber(5)),
	), makeSearchHandler(searchEngine.BookSearch, "book"))

	return s
}

// makeSearchHandler creates an MCP tool handler from any search function.
func makeSearchHandler(
	searchFn func(context.Context, string, int) ([]models.SearchResult, error),
	resultType string,
) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		maxResults := request.GetInt("max_results", 5)

		slog.Info("executing search", "type", resultType, "query", query, "maxResults", maxResults)
		results, err := searchFn(ctx, query, maxResults)
		if err != nil {
			slog.Error("search failed", "type", resultType, "query", query, "error", err)
			return mcp.NewToolResultError(err.Error()), nil
		}

		result, err := mcp.NewToolResultJSON(models.SearchResponse{
			Type:    resultType,
			Results: results,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return result, nil
	}
}

