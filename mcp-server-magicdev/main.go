package main

import (
	"log/slog"
	"os"

	"mcp-server-magicdev/cmd"
)

func main() {
	// Initialize standard stderr logger for baseline
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if err := cmd.Execute(); err != nil {
		slog.Error("fatal error", "err", err)
		os.Exit(1)
	}
}
