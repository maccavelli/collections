package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Destructively clears the underlying datastore",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := Cfg.GetDBPath()
		slog.Warn("purge requested: deleting existing database", "path", dbPath)
		if dbPath == "" || dbPath == "/" || dbPath == "." {
			return fmt.Errorf("refusing to reinit: invalid or dangerous database path: %s", dbPath)
		}
		if err := os.RemoveAll(dbPath); err != nil {
			return fmt.Errorf("failed to clear database during purge: %w", err)
		}
		slog.Info("database reset successfully. Exiting.")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(purgeCmd)
}
