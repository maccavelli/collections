package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Custom usage for --version
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  --version    Print the version and exit\n")
	}
	versionFlag := flag.Bool("version", false, "Print the version and exit")
	flag.Parse()

	if *versionFlag {
		printVersion()
		os.Exit(0)
	}

	// Create MCP server
	s := server.NewMCPServer(
		"DuckDuckGo Search",
		Version,
		server.WithLogging(),
	)

	engine := NewSearchEngine()

	// Register tools
	s.AddTool(mcp.NewTool("ddg_search_web",
		mcp.WithDescription("Perform a high-quality web search using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), makeSearchHandler(engine.WebSearch))

	s.AddTool(mcp.NewTool("ddg_search_news",
		mcp.WithDescription("Perform a news-specific search using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), makeSearchHandler(engine.NewsSearch))

	s.AddTool(mcp.NewTool("ddg_search_images",
		mcp.WithDescription("Search for images using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), makeSearchHandler(engine.ImageSearch))

	s.AddTool(mcp.NewTool("ddg_search_videos",
		mcp.WithDescription("Search for videos using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), makeSearchHandler(engine.VideoSearch))

	s.AddTool(mcp.NewTool("ddg_search_books",
		mcp.WithDescription("Search for books using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), makeSearchHandler(engine.BookSearch))

	// Start the server using stdio transport
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
		os.Exit(1)
	}
}

// makeSearchHandler creates an MCP tool handler from any search function.
// This eliminates the duplicated boilerplate across all 5 search handlers.
func makeSearchHandler(
	searchFn func(string, int) ([]SearchResult, error),
) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		maxResults := request.GetInt("max_results", 5)

		results, err := searchFn(query, maxResults)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result, err := mcp.NewToolResultJSON(SearchResponse{
			Query:   query,
			Results: results,
			Count:   len(results),
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return result, nil
	}
}
