package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-duckduckgo/internal/config"
	"mcp-server-duckduckgo/internal/engine"
	"mcp-server-duckduckgo/internal/handler/media"
	"mcp-server-duckduckgo/internal/handler/search"
	"mcp-server-duckduckgo/internal/registry"
)

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("%s version %s\n", config.Name, Version)
		os.Exit(0)
	}

	setupLogging()
	slog.Info("starting "+config.Name, "version", Version)

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
	eng := engine.NewSearchEngine()

	// Tool Registration
	search.Register(eng)
	media.Register(eng)

	// Setup MCP Server using official SDK
	mcpSrv := mcp.NewServer(
		&mcp.Implementation{
			Name:    config.Platform + " Search",
			Version: Version,
		},
		&mcp.ServerOptions{
			// Logging is default to stderr in slog
		},
	)

	// Register tools from global registry using the new pattern
	for _, t := range registry.Global.List() {
		t.Register(mcpSrv)
	}

	return mcpSrv.Run(ctx, &mcp.StdioTransport{})
}
