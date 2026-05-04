// Package cmd implements the Cobra command tree for the MagicDev MCP server.
package cmd

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags -X main.Version=...
// and propagated here for the MCP Implementation metadata.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "magicdev",
	Short: "MagicDev is a stateful MCP server for .NET/Node requirements engineering",
	Long:  `MagicDev provides an "Idea-to-Asset" pipeline for technical planning, integrated with BuntDB, Atlassian, and GitLab.`,
}

// Execute runs the root command tree. Called from main().
func Execute() error {
	return rootCmd.Execute()
}
