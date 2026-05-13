package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set from main.go ldflags
var Version string = "dev"

var rootCmd = &cobra.Command{
	Use:   "socratic-thinker",
	Short: "Socratic Thinker MCP Server",
	Long:  `Socratic Thinker MCP Server provides deep cognitive processing and paradox resolution via the Model Context Protocol.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to serve if no args
		serveCmd.Run(serveCmd, args)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(dashboardCmd)
}
