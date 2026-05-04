// Package cmd implements the Cobra command tree for the MagicDev MCP server.
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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

		// Load YAML config and init fsnotify.
		if err := config.LoadConfig(); err != nil {
			slog.Warn("could not load magicdev.yaml config, proceeding with defaults", "err", err)
		}

		// Backward compatibility bindings for Kubernetes / Legacy environments
		viper.BindEnv("atlassian.url", "ATLASSIAN_URL")
		viper.BindEnv("atlassian.token", "ATLASSIAN_TOKEN")
		viper.BindEnv("git.username", "GIT_USERNAME")
		viper.BindEnv("git.token", "GIT_TOKEN")
		viper.BindEnv("server.db_path", "MAGICDEV_DB_PATH")

		// Security & Environment Parameters validation hook
		if viper.GetString("atlassian.token") == "" || viper.GetString("atlassian.token") == "PLACEHOLDER_ATLASSIAN_TOKEN" {
			slog.Warn("Atlassian integration missing token in magicdev.yaml")
		}
		if viper.GetString("git.token") == "" || viper.GetString("git.token") == "PLACEHOLDER_GIT_TOKEN" {
			slog.Warn("Git integration missing token in magicdev.yaml")
		}

		store, err := db.InitStore()
		if err != nil {
			return fmt.Errorf("failed to initialize session store: %w", err)
		}
		defer store.Close()

		handler.RegisterTools(s, store)
		handler.RegisterPrompts(s)

		slog.Info("MCP server ready", "version", Version)
		return s.Run(context.Background(), &mcp.StdioTransport{})
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
