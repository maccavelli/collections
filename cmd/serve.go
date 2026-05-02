package cmd

import (
	"context"
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
	Run: func(cmd *cobra.Command, args []string) {
		slog.Info("starting magicdev MCP server")

		// Create MCP server
		s := mcp.NewServer(&mcp.Implementation{
			Name:    "magicdev",
			Version: "1.0.0",
		}, nil)

		// Initialize Config
		if _, err := config.LoadConfig(); err != nil {
			slog.Warn("could not load encrypted config, proceeding without credentials", "err", err)
		}

		// Initialize DB
		store, err := db.InitStore()
		if err != nil {
			slog.Error("failed to initialize session store", "err", err)
			return
		}
		defer store.Close()

		// Register Tools
		handler.RegisterTools(s, store)

		// Start server
		if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			slog.Error("server error", "err", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
