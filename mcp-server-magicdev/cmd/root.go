package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "magicdev",
	Short: "MagicDev is a stateful MCP server for .NET/Node requirements engineering",
	Long:  `MagicDev provides an "Idea-to-Asset" pipeline for technical planning, integrated with BuntDB, Atlassian, and GitLab.`,
}

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Root command flags (if any)
}
