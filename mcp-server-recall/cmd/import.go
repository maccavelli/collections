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

var importCmd = &cobra.Command{
	Use:     "import [filepath]",
	Short:   "Imports a JSONL backup file into the running recall database",
	Long:    "Reads a JSONL file previously created by 'export' and restores all records (memories, standards, sessions) into the running Recall server. Preserves original timestamps and metadata.",
	Example: `  mcp-server-recall import ./backup.jsonl`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port := Cfg.APIPort()
		if port == 0 {
			port = 7000
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

		fmt.Fprintf(os.Stderr, "Connected. Importing database from %s\n", args[0])

		toolArgs := map[string]interface{}{
			"filename": args[0],
		}

		res, err := mcpClient.CallDatabaseTool(ctx, "import_memories", toolArgs)
		if err != nil {
			return fmt.Errorf("import_memories: %w", err)
		}

		fmt.Fprintln(RealStdout, res)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(importCmd)
}
