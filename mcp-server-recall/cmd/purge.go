package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"mcp-server-recall/internal/memory"
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

	for _, domain := range []string{"memories", "standards", "projects", "sessions"} {
		domainCopy := domain
		subCmd := &cobra.Command{
			Use:   domainCopy,
			Short: fmt.Sprintf("Destructively purges the %s namespace exclusively", domainCopy),
			RunE: func(cmd *cobra.Command, args []string) error {
				ctx := context.Background()
				store, err := memory.NewMemoryStore(ctx, Cfg.GetDBPath(), Cfg.EncryptionKey(), Cfg.SearchLimit(), Cfg.BatchSettings())
				if err != nil {
					return err
				}
				defer store.Close()

				deleted, err := store.PurgeDomain(ctx, domainCopy)
				if err != nil {
					return err
				}
				slog.Info("Purged namespace", "domain", domainCopy, "deleted_count", deleted)
				return nil
			},
		}
		purgeCmd.AddCommand(subCmd)
	}
}
