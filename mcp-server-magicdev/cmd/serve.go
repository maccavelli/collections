// Package cmd implements the Cobra command tree for the MagicDev MCP server.
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"mcp-server-magicdev/internal/config"
	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/handler"
	"mcp-server-magicdev/internal/sync"
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
		viper.BindEnv("confluence.url", "CONFLUENCE_URL")
		viper.BindEnv("jira.url", "JIRA_URL")
		viper.BindEnv("git.username", "GIT_USERNAME")
		viper.BindEnv("git.server_url", "GITLAB_URL")
		viper.BindEnv("git.project_path", "GITLAB_PROJECT_PATH")
		viper.BindEnv("server.db_path", "MAGICDEV_DB_PATH")

		// Apply Runtime Optimizations
		if memLimit := viper.GetString("runtime.gomemlimit"); memLimit != "" {
			memLimit = strings.ToUpper(strings.TrimSpace(memLimit))
			var limitBytes int64
			if strings.HasSuffix(memLimit, "GB") {
				val, err := strconv.ParseInt(strings.TrimSuffix(memLimit, "GB"), 10, 64)
				if err == nil {
					limitBytes = val * 1024 * 1024 * 1024
				}
			} else if strings.HasSuffix(memLimit, "MB") {
				val, err := strconv.ParseInt(strings.TrimSuffix(memLimit, "MB"), 10, 64)
				if err == nil {
					limitBytes = val * 1024 * 1024
				}
			}
			if limitBytes > 0 {
				debug.SetMemoryLimit(limitBytes)
				slog.Info("applied soft memory limit", "bytes", limitBytes, "config", memLimit)
			}
		}

		if maxProcs := viper.GetString("runtime.gomaxprocs"); maxProcs != "" {
			val, err := strconv.Atoi(strings.TrimSpace(maxProcs))
			if err == nil && val > 0 {
				runtime.GOMAXPROCS(val)
				slog.Info("applied max procs limit", "threads", val)
			}
		}

		store, err := db.InitStore()
		if err != nil {
			return fmt.Errorf("failed to initialize session store: %w", err)
		}
		defer store.Close()

		// Auto-provisioning logic via environment variables
		provisionVault(store, "gitlab", "GITLAB_USER_TOKEN")
		provisionVault(store, "jira", "JIRA_USER_TOKEN")
		provisionVault(store, "confluence", "CONFLUENCE_USER_TOKEN")

		// Security & Environment Parameters validation hook
		checkVaultSecret(store, "confluence")
		checkVaultSecret(store, "jira")
		checkVaultSecret(store, "gitlab")

		// Launch the background baseline standards sync priority cascade
		go sync.SyncBaselines(store)

		handler.RegisterTools(s, store)
		handler.RegisterPrompts(s)

		slog.Info("MCP server ready", "version", Version)
		return s.Run(context.Background(), &mcp.StdioTransport{})
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func provisionVault(store *db.Store, service, envKey string) {
	if envVal := os.Getenv(envKey); envVal != "" {
		if err := store.SetSecret(service, envVal); err != nil {
			slog.Error("failed to auto-provision vault", "service", service, "err", err)
		} else {
			slog.Info("auto-provisioned vault secret from environment", "service", service)
		}
	}
}

func checkVaultSecret(store *db.Store, service string) {
	val, err := store.GetSecret(service)
	if err != nil || val == "" {
		slog.Warn("missing secret in vault", "service", service, "action", "run 'mcp-server-magicdev token update' to configure")
	}
}
