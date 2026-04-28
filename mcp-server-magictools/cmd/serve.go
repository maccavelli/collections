package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// ServeFunc is a callback to the main server loop defined in main.go
// This avoids circular dependencies while keeping the logic in main.go for now.
var ServeFunc func(ctx context.Context, configPath, dbPath, logPath, logLevel string, noOptimize, debug bool) error

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server (default)",
	RunE: func(cmd *cobra.Command, args []string) error {
		HijackStdout()
		return ServeFunc(cmd.Context(), CfgPath, DBPath, LogPath, LogLevelFlag, NoOptimize, Debug)
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	// Make serve the default if no subcommand is provided
	// We wrap it to ensure we don't hijack if --version is requested via legacy flag
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if ShowVersion {
			fmt.Printf("mcp-server-magictools %s\n", Version)
			return nil
		}
		return serveCmd.RunE(cmd, args)
	}
}
