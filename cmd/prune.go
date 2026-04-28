package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/spf13/cobra"
	"mcp-server-recall/internal/memory"
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

		ctx := context.Background()
		store, err := memory.NewMemoryStore(ctx, Cfg.GetDBPath(), Cfg.EncryptionKey(), Cfg.SearchLimit(), Cfg.BatchSettings())
		if err != nil {
			return err
		}
		defer store.Close()

		deleted, err := store.PruneDomain(ctx, "", days)
		if err != nil {
			return err
		}
		slog.Info("Global prune complete", "days_older_than", days, "deleted_count", deleted)
		return nil
	},
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

				ctx := context.Background()
				store, err := memory.NewMemoryStore(ctx, Cfg.GetDBPath(), Cfg.EncryptionKey(), Cfg.SearchLimit(), Cfg.BatchSettings())
				if err != nil {
					return err
				}
				defer store.Close()

				deleted, err := store.PruneDomain(ctx, domainCopy, days)
				if err != nil {
					return err
				}
				slog.Info("Pruned namespace", "domain", domainCopy, "days_older_than", days, "deleted_count", deleted)
				return nil
			},
		}
		pruneCmd.AddCommand(subCmd)
	}
}
