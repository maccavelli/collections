// Package cmd implements the Cobra command tree for the MagicDev MCP server.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"mcp-server-magicdev/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate the default magicdev.yaml configuration file and exit",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Initializing MagicDev Configuration...")

		created, err := config.EnsureConfig()
		if err != nil {
			return fmt.Errorf("failed to ensure config: %w", err)
		}

		if !created {
			fmt.Println("Configuration file already exists.")
		} else {
			path, _ := config.ConfigPath()
			fmt.Printf("Default configuration successfully generated at %s\n", path)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
