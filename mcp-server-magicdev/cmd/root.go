// Package cmd implements the Cobra command tree for the MagicDev MCP server.
package cmd

import (
	"github.com/spf13/cobra"
)

// Version is propagated from the main package for MCP Implementation metadata.
var Version string

var rootCmd = &cobra.Command{
	Use:     "magicdev",
	Version: Version,
	Short:   "MagicDev is a stateful MCP server for .NET/Node requirements engineering",
	Long:    `MagicDev provides an "Idea-to-Asset" pipeline for technical planning, integrated with BuntDB, Atlassian, and GitLab.`,
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
}

// Execute runs the root command tree. Called from main().
func Execute() error {
	rootCmd.Version = Version
	return rootCmd.Execute()
}
