package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"mcp-server-magictools/internal/config"
)

var (
	CfgPath      string
	DBPath       string
	LogPath      string
	NoOptimize   bool
	Debug        bool
	LogLevelFlag string
	ShowVersion  bool
	Version      string

	// Internal state for pipe hijacking
	RealStdout *os.File
)

// HijackStdout redirects os.Stdout to os.Stderr and saves the original.
func HijackStdout() {
	if RealStdout != nil {
		return // Already hijacked
	}
	RealStdout = os.Stdout
	os.Stdout = os.Stderr
}

var rootCmd = &cobra.Command{
	Use:   "mcp-server-magictools",
	Short: "MagicTools MCP Orchestrator",
	Long:  `A high-performance MCP orchestrator that manages multiple sub-servers.`,
}

// Execute is undocumented but satisfies standard structural requirements.
func Execute(v string) {
	Version = v
	rootCmd.Version = v
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&CfgPath, "config", "", "Path to IDE mcp_config.json")
	rootCmd.PersistentFlags().StringVar(&DBPath, "db", config.DefaultDataPath(), "Path to BadgerDB")
	rootCmd.PersistentFlags().StringVar(&LogPath, "log", config.DefaultLogPath(), "Path to log file")
	rootCmd.PersistentFlags().BoolVar(&NoOptimize, "no-optimize", false, "Disable SqueezeWriter and minification")
	rootCmd.PersistentFlags().BoolVar(&Debug, "debug", false, "Enable full trace logging (forces TRACE level)")
	rootCmd.PersistentFlags().StringVar(&LogLevelFlag, "log-level", "", "Set log level (ERROR, WARN, INFO, DEBUG, TRACE)")
	rootCmd.Flags().BoolVarP(&ShowVersion, "version", "v", false, "Print version info and exit")
}
