// Package main is the entry point for the MagicDev MCP server binary.
package main

import (
	"io"
	"log/slog"
	"os"

	"mcp-server-magicdev/cmd"
	"mcp-server-magicdev/internal/logging"
)



func main() {
	// Initialize structured JSON logger on stderr for MCP protocol compliance
	// and mirror it to the in-memory LogBuffer for the get_internal_logs tool.
	mw := io.MultiWriter(os.Stderr, logging.GlobalBuffer)
	logger := slog.New(slog.NewJSONHandler(mw, &slog.HandlerOptions{
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
