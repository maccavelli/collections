package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"mcp-server-duckduckgo/internal/engine"
	"mcp-server-duckduckgo/internal/models"
)

func main() {
	if err := run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("mcp-server-duckduckgo", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", args[0])
		fmt.Fprintf(os.Stderr, "  --version    Print the version and exit\n")
	}
	versionFlag := fs.Bool("version", false, "Print the version and exit")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	if *versionFlag {
		printVersion()
		return nil
	}

	s := newServer(engine.NewSearchEngine())
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
		mcp.WithDescription("Perform a high-quality web search using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), makeSearchHandler(searchEngine.WebSearch, "web"))

	s.AddTool(mcp.NewTool("ddg_search_news",
		mcp.WithDescription("Perform a news-specific search using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), makeSearchHandler(searchEngine.NewsSearch, "news"))

	s.AddTool(mcp.NewTool("ddg_search_images",
		mcp.WithDescription("Search for images using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), makeSearchHandler(searchEngine.ImageSearch, "image"))

	s.AddTool(mcp.NewTool("ddg_search_videos",
		mcp.WithDescription("Search for videos using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), makeSearchHandler(searchEngine.VideoSearch, "video"))

	s.AddTool(mcp.NewTool("ddg_search_books",
		mcp.WithDescription("Search for books using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
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

		results, err := searchFn(ctx, query, maxResults)
		if err != nil {
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

