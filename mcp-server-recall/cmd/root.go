package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"mcp-server-recall/internal/config"
)

var (
	// Version is injected during runtime
	Version = "dev"

	// Cfg provides thread-safe access to Viper values
	Cfg *config.Config

	// RealStdout carefully preserves standard out for JSON-RPC transport constraints
	RealStdout *os.File
)

// RootCmd intercepts all runtime behaviors cleanly
var RootCmd = &cobra.Command{
	Use:   config.Name,
	Short: "Recall Engine",
	Long:  "A Model Context Protocol orchestration engine for codebase extraction and vector recall.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return serveCmd.RunE(cmd, args)
	},
}

// Execute protects the native standard streams before cascading into subcommands.
func Execute(version string) {
	Version = version

	// CRITICAL constraint: Steal os.Stdout to forbid Cobra usage-printing corruption
	RealStdout = os.Stdout
	os.Stdout = os.Stderr
	RootCmd.SetOut(os.Stderr)
	RootCmd.SetErr(os.Stderr)

	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal execution error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	// Initialize Viper mappings and spawn the fsnotify file-watcher loops securely
	Cfg = config.New(Version)
}
