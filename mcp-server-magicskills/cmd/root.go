package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"mcp-server-magicskills/internal/config"
)

var (
	DBPath       string
	LogPath      string
	NoOptimize   bool
	Debug        bool
	LogLevelFlag string
	ShowVersion  bool
	Version      string

	RealStdout *os.File
)

func HijackStdout() {
	if RealStdout != nil {
		return
	}
	RealStdout = os.Stdout
	os.Stdout = os.Stderr
}

var rootCmd = &cobra.Command{
	Use:   "mcp-server-magicskills",
	Short: "MagicSkills MCP Sub-Server",
	Long:  `A high-performance MCP sub-server managing executable magic skills.`,
}

// Execute is the main entry point called by main.go.
func Execute(v string) {
	Version = v
	rootCmd.Version = v
	rootCmd.FParseErrWhitelist.UnknownFlags = true
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&DBPath, "db", config.ResolveDataDir(), "Path to BadgerDB")
	rootCmd.PersistentFlags().StringVar(&LogPath, "log", "", "Path to log file")
	rootCmd.PersistentFlags().BoolVar(&NoOptimize, "no-optimize", false, "Disable SqueezeWriter and minification")
	rootCmd.PersistentFlags().BoolVar(&Debug, "debug", false, "Enable full trace logging (forces TRACE level)")
	rootCmd.PersistentFlags().StringVar(&LogLevelFlag, "log-level", "", "Set log level (ERROR, WARN, INFO, DEBUG, TRACE)")
	rootCmd.Flags().BoolVarP(&ShowVersion, "version", "v", false, "Print version info and exit")
}
