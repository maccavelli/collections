package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"mcp-server-recall/internal/client"
)

var harvestCmd = &cobra.Command{
	Use:   "harvest",
	Short: "Harvest structural intelligence from Go packages into Recall namespaces",
	Example: `  mcp-server-recall harvest standards github.com/ollama/ollama/api
  mcp-server-recall harvest projects /path/to/local/project`,
}

var harvestStandardsCmd = &cobra.Command{
	Use:     "standards [package-path]",
	Short:   "Harvests a Go package into the standards namespace (external libraries)",
	Example: `  mcp-server-recall harvest standards github.com/ollama/ollama/api`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHarvestViaMCP("standards", args[0])
	},
}

var harvestProjectsCmd = &cobra.Command{
	Use:   "projects [package-path]",
	Short: "Harvests a local Go project into the projects namespace (project intelligence)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHarvestViaMCP("projects", args[0])
	},
}

// runHarvestViaMCP connects to the running Recall MCP server and calls the specified harvest tool.
func runHarvestViaMCP(namespace, pkgPath string) error {
	port := Cfg.APIPort()
	if port == 0 {
		port = 7000
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	url := fmt.Sprintf("http://127.0.0.1:%d/mcp/internal", port)
	fmt.Fprintf(os.Stderr, "Connecting to local recall server at %s...\n", url)

	mcpClient := client.NewMCPClient(url)
	go mcpClient.Start(ctx)

	// Blocking wait until the 'initialize' lifecycle is fully established
	for {
		if mcpClient.RecallEnabled() {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	fmt.Fprintf(os.Stderr, "Connected to running Recall server. Firing %s -> %s\n", namespace, pkgPath)

	toolArgs := map[string]any{
		"namespace":   namespace,
		"target_path": pkgPath,
	}

	toolName := "harvest"
	res, err := mcpClient.CallDatabaseTool(ctx, toolName, toolArgs)
	if err != nil {
		return fmt.Errorf("%s: %w", namespace, err)
	}

	// Push the raw structured JSON payload out correctly
	fmt.Fprintln(RealStdout, res)
	return nil
}

func init() {
	harvestCmd.AddCommand(harvestStandardsCmd, harvestProjectsCmd)
	RootCmd.AddCommand(harvestCmd)
}
