package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"mcp-server-recall/internal/client"
)

var pruneCmd = &cobra.Command{
	Use:   "prune [days]",
	Short: "Prunes records older than [days] across all namespaces (default 30)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		days := 30
		if len(args) > 0 {
			var err error
			days, err = strconv.Atoi(args[0])
			if err != nil || days < 0 {
				return fmt.Errorf("invalid days argument: %s", args[0])
			}
		}
		return runPruneViaMCP("all", days)
	},
}

// runPruneViaMCP connects to the running Recall MCP server and calls the prune_records tool.
func runPruneViaMCP(namespace string, days int) error {
	port := Cfg.APIPort()
	if port == 0 {
		port = 18001
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	url := fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
	fmt.Fprintf(os.Stderr, "Connecting to local recall server at %s...\n", url)

	mcpClient := client.NewMCPClient(url)
	go mcpClient.Start(ctx)

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

	fmt.Fprintf(os.Stderr, "Connected. Pruning namespace: %s (older than %d days)\n", namespace, days)

	toolArgs := map[string]any{
		"namespace": namespace,
		"days_old":  days,
	}

	res, err := mcpClient.CallDatabaseTool(ctx, "prune_records", toolArgs)
	if err != nil {
		return fmt.Errorf("prune_records(%s): %w", namespace, err)
	}

	fmt.Fprintln(RealStdout, res)
	return nil
}

func init() {
	RootCmd.AddCommand(pruneCmd)

	for _, domain := range []string{"memories", "standards", "projects", "sessions"} {
		domainCopy := domain
		subCmd := &cobra.Command{
			Use:   fmt.Sprintf("%s [days]", domainCopy),
			Short: fmt.Sprintf("Prunes %s records older than [days] (default 30)", domainCopy),
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				days := 30
				if len(args) > 0 {
					var err error
					days, err = strconv.Atoi(args[0])
					if err != nil || days < 0 {
						return fmt.Errorf("invalid days argument: %s", args[0])
					}
				}
				return runPruneViaMCP(domainCopy, days)
			},
		}
		pruneCmd.AddCommand(subCmd)
	}
}
