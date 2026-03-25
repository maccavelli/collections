package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"mcp-server-recall/internal/config"
	"mcp-server-recall/internal/memory"
	"mcp-server-recall/internal/server"
)

func main() {
	cfg := config.New()
	setupLogging()
	slog.Info("starting "+cfg.Name, "version", cfg.Version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg); err != nil {
		slog.Error("server fatal error", "error", err)
		os.Exit(1)
	}
	slog.Info("server shutdown complete")
}

func setupLogging() {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))
}

func run(ctx context.Context, cfg *config.Config) error {
	// Initialize the storage layer
	store, err := memory.NewMemoryStore(cfg.GetDBPath())
	if err != nil {
		return fmt.Errorf("could not launch MemoryStore: %w", err)
	}
	defer store.Close()

	// Initialize the MCP server
	mcpServer, err := server.NewMCPRecallServer(cfg.Name, cfg.Version, store)
	if err != nil {
		return fmt.Errorf("could not launch MCP server: %w", err)
	}

	// Channel to signal server exit
	errChan := make(chan error, 1)

	go func() {
		// Run the server on stdio transport
		if err := mcpServer.Serve(ctx); err != nil {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		slog.Info("context cancelled; initiating graceful shutdown")
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
