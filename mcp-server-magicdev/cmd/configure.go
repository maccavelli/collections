// Package cmd implements the Cobra command tree for the MagicDev MCP server.
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"mcp-server-magicdev/internal/config"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Initialize the magicdev.yaml configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Initializing MagicDev Configuration...")
		
		created, err := config.EnsureConfig()
		if err != nil {
			return fmt.Errorf("failed to ensure config: %w", err)
		}
		
		if !created {
			fmt.Println("Configuration file already exists.")
			fmt.Print("Do you want to overwrite it? (y/N): ")
			reader := bufio.NewReader(os.Stdin)
			ans, _ := reader.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(ans)) != "y" {
				fmt.Println("Aborting.")
				return nil
			}
		}

		reader := bufio.NewReader(os.Stdin)

		fmt.Print("Atlassian URL (e.g. https://your-domain.atlassian.net): ")
		atlassianURL, _ := reader.ReadString('\n')
		atlassianURL = strings.TrimSpace(atlassianURL)

		fmt.Print("Atlassian Token: ")
		atlassianToken, _ := reader.ReadString('\n')
		atlassianToken = strings.TrimSpace(atlassianToken)

		fmt.Print("Git Username: ")
		gitUser, _ := reader.ReadString('\n')
		gitUser = strings.TrimSpace(gitUser)

		fmt.Print("Git Token: ")
		gitToken, _ := reader.ReadString('\n')
		gitToken = strings.TrimSpace(gitToken)

		template := config.DefaultConfigTemplate
		if atlassianURL != "" {
			template = strings.Replace(template, `"PLACEHOLDER_ATLASSIAN_URL"`, fmt.Sprintf(`"%s"`, atlassianURL), 1)
		}
		if atlassianToken != "" {
			template = strings.Replace(template, `"PLACEHOLDER_ATLASSIAN_TOKEN"`, fmt.Sprintf(`"%s"`, atlassianToken), 1)
		}
		if gitUser != "" {
			template = strings.Replace(template, `"PLACEHOLDER_GIT_USERNAME"`, fmt.Sprintf(`"%s"`, gitUser), 1)
		}
		if gitToken != "" {
			template = strings.Replace(template, `"PLACEHOLDER_GIT_TOKEN"`, fmt.Sprintf(`"%s"`, gitToken), 1)
		}

		path, err := config.ConfigPath()
		if err != nil {
			return err
		}

		if err := os.WriteFile(path, []byte(template), 0600); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}

		fmt.Printf("Configuration successfully saved to %s\n", path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configureCmd)
}
