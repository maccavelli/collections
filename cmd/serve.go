// Package cmd implements the Cobra command tree for the MagicDev MCP server.
package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"mcp-server-magicdev/internal/config"
	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/handler"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MagicDev MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Info("starting magicdev MCP server")

		// Create MCP server.
		s := mcp.NewServer(&mcp.Implementation{
			Name:    "magicdev",
			Version: Version,
		}, nil)

		// Load encrypted credentials into viper.
		if _, err := config.LoadConfig(); err != nil {
			slog.Warn("could not load encrypted config, proceeding without credentials", "err", err)
		}

		store, err := db.InitStore()
		if err != nil {
			return fmt.Errorf("failed to initialize session store: %w", err)
		}
		defer store.Close()

		handler.RegisterTools(s, store)

		slog.Info("MCP server ready", "version", Version)
		return s.Run(context.Background(), &mcp.StdioTransport{})
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
