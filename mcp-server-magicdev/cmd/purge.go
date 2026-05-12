// Package cmd implements the Cobra command tree for the MagicDev MCP server.
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"mcp-server-magicdev/internal/db"
)

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Purge data from the BuntDB database",
	Example: `  mcp-server-magicdev purge sessions
  mcp-server-magicdev purge baselines
  mcp-server-magicdev purge chaos`,
}

var purgeSessionsCmd = &cobra.Command{
	Use:     "sessions",
	Short:   "Purge all session data from the database",
	Example: `  mcp-server-magicdev purge sessions`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print("Purge all sessions? (y/N): ")
		reader := bufio.NewReader(os.Stdin)
		ans, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}

		store, err := db.InitStore()
		if err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer store.Close()

		count, err := store.PurgeSessions()
		if err != nil {
			return fmt.Errorf("failed to purge sessions: %w", err)
		}

		if count == 0 {
			fmt.Println("No sessions found to purge.")
		} else {
			fmt.Printf("Purged %d sessions.\n", count)
		}
		return nil
	},
}

var purgeBaselinesCmd = &cobra.Command{
	Use:     "baselines",
	Short:   "Purge all cached baseline standards from the database",
	Example: `  mcp-server-magicdev purge baselines`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print("Purge all baselines? (y/N): ")
		reader := bufio.NewReader(os.Stdin)
		ans, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}

		store, err := db.InitStore()
		if err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer store.Close()

		count, err := store.PurgeBaselines()
		if err != nil {
			return fmt.Errorf("failed to purge baselines: %w", err)
		}

		if count == 0 {
			fmt.Println("No baselines found to purge.")
		} else {
			fmt.Printf("Purged %d baselines.\n", count)
		}
		return nil
	},
}

var purgeChaosCmd = &cobra.Command{
	Use:     "chaos",
	Short:   "Purge all cached chaos graveyard patterns from the database",
	Example: `  mcp-server-magicdev purge chaos`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print("Purge all chaos graveyards? (y/N): ")
		reader := bufio.NewReader(os.Stdin)
		ans, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}

		store, err := db.InitStore()
		if err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer store.Close()

		count, err := store.PurgeChaosGraveyards()
		if err != nil {
			return fmt.Errorf("failed to purge chaos graveyards: %w", err)
		}

		if count == 0 {
			fmt.Println("No chaos graveyard patterns found to purge.")
		} else {
			fmt.Printf("Purged %d chaos graveyard patterns.\n", count)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(purgeCmd)
	purgeCmd.AddCommand(purgeSessionsCmd)
	purgeCmd.AddCommand(purgeBaselinesCmd)
	purgeCmd.AddCommand(purgeChaosCmd)
}
