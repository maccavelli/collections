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

var exportCmd = &cobra.Command{
	Use:     "export [filepath]",
	Short:   "Exports the entire recall database to a JSONL file for backup",
	Long:    "Streams all records (memories, standards, sessions) from the running Recall server to a JSONL file. The output file must not already exist (O_EXCL safety).",
	Example: `  mcp-server-recall export ./backup.jsonl`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
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

		fmt.Fprintf(os.Stderr, "Connected. Exporting database to %s\n", args[0])

		toolArgs := map[string]any{
			"filename": args[0],
		}

		res, err := mcpClient.CallDatabaseTool(ctx, "export_records", toolArgs)
		if err != nil {
			return fmt.Errorf("export_records: %w", err)
		}

		fmt.Fprintln(RealStdout, res)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(exportCmd)
}
