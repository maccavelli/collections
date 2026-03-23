package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/server"
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

	s := server.NewMCPServer(
		config.Platform+" Search",
		Version,
		server.WithLogging(),
	)

	// Register tools from global registry
	for _, t := range registry.Global.List() {
		s.AddTool(t.Metadata(), t.Handle)
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutdown signal received; stopping server")
	}()

	return server.ServeStdio(s)
}
