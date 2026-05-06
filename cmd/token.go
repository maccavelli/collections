// Package cmd provides functionality for the cmd subsystem.
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"mcp-server-magicdev/internal/db"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage secure tokens in the BuntDB vault",
	Long:  `The token command allows you to interactively update or list securely stored tokens (GitLab, Confluence, Jira).`,
	Example: `  mcp-server-magicdev token list
  mcp-server-magicdev token reconfigure`,
}


var tokenListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List stored secure tokens (plaintext)",
	Example: `  mcp-server-magicdev token list`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.InitStore()
		if err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer store.Close()

		services := []string{"gitlab", "confluence", "jira"}
		fmt.Println("Stored Tokens:")
		for _, svc := range services {
			token, err := store.GetSecret(svc)
			if err != nil {
				fmt.Printf("- %s: ERROR (%v)\n", svc, err)
			} else if token == "" {
				fmt.Printf("- %s: (Not Set)\n", svc)
			} else {
				fmt.Printf("- %s: %s\n", svc, token)
			}
		}
		return nil
	},
}

var tokenReconfigureCmd = &cobra.Command{
	Use:   "reconfigure",
	Short: "Reconfigure tokens from environment variables or interactive prompt",
	Example: `  mcp-server-magicdev token reconfigure
  GITLAB_USER_TOKEN=xyz mcp-server-magicdev token reconfigure`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.InitStore()
		if err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer store.Close()

		reader := bufio.NewReader(os.Stdin)

		services := []struct {
			name   string
			envVar string
		}{
			{"gitlab", "GITLAB_USER_TOKEN"},
			{"confluence", "CONFLUENCE_USER_TOKEN"},
			{"jira", "JIRA_USER_TOKEN"},
		}

		for _, svc := range services {
			val := os.Getenv(svc.envVar)
			if val != "" {
				if err := store.SetSecret(svc.name, val); err != nil {
					return fmt.Errorf("failed to save %s token: %w", svc.name, err)
				}
				fmt.Printf("Imported %s token from %s\n", svc.name, svc.envVar)
			} else {
				fmt.Printf("Enter %s Token: ", svc.name)
				tokenStr, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read input: %w", err)
				}
				tokenStr = strings.TrimSpace(tokenStr)
				if tokenStr != "" {
					if err := store.SetSecret(svc.name, tokenStr); err != nil {
						return fmt.Errorf("failed to save %s token: %w", svc.name, err)
					}
					fmt.Printf("Successfully updated %s token.\n", svc.name)
				} else {
					fmt.Printf("Skipped %s token (empty input).\n", svc.name)
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tokenCmd)
	tokenCmd.AddCommand(tokenListCmd)
	tokenCmd.AddCommand(tokenReconfigureCmd)
}
