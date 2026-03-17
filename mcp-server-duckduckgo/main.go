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
	), webSearchHandler(engine))

	s.AddTool(mcp.NewTool("ddg_search_news",
		mcp.WithDescription("Perform a news-specific search using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), newsSearchHandler(engine))

	s.AddTool(mcp.NewTool("ddg_search_images",
		mcp.WithDescription("Search for images using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), imageSearchHandler(engine))

	s.AddTool(mcp.NewTool("ddg_search_videos",
		mcp.WithDescription("Search for videos using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), videoSearchHandler(engine))

	s.AddTool(mcp.NewTool("ddg_search_books",
		mcp.WithDescription("Search for books using DuckDuckGo."),
		mcp.WithString("query", mcp.Description("The search keywords"), mcp.Required()),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 5)"), mcp.DefaultNumber(5)),
	), bookSearchHandler(engine))

	// Start the server using stdio transport
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
		os.Exit(1)
	}
}

func webSearchHandler(engine *SearchEngine) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		maxResults := request.GetInt("max_results", 5)

		results, err := engine.WebSearch(query, maxResults)
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

func newsSearchHandler(engine *SearchEngine) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		maxResults := request.GetInt("max_results", 5)

		results, err := engine.NewsSearch(query, maxResults)
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

func imageSearchHandler(engine *SearchEngine) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		maxResults := request.GetInt("max_results", 5)

		results, err := engine.ImageSearch(query, maxResults)
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

func videoSearchHandler(engine *SearchEngine) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		maxResults := request.GetInt("max_results", 5)

		results, err := engine.VideoSearch(query, maxResults)
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

func bookSearchHandler(engine *SearchEngine) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		fmt.Fprintf(os.Stderr, "DEBUG: BookSearch version 1.0.1 called for query: %s\n", query)
		maxResults := request.GetInt("max_results", 5)

		results, err := engine.BookSearch(query, maxResults)
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
