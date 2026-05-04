// Package main is the entry point for the MagicDev MCP server binary.
package main

import (
	"log/slog"
	"os"

	"mcp-server-magicdev/cmd"
)

// Version is injected at build time via ldflags.
var Version = "dev"

func main() {
	// Initialize structured JSON logger on stderr for MCP protocol compliance
	// (stdout is reserved for the stdio transport).
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Propagate ldflags version to the cmd package.
	cmd.Version = Version

	if err := cmd.Execute(); err != nil {
		slog.Error("fatal error", "err", err)
		os.Exit(1)
	}
}
