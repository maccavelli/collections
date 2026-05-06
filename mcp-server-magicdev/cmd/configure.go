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

		fmt.Print("Confluence URL (e.g. https://your-domain.atlassian.net/wiki): ")
		confluenceURL, _ := reader.ReadString('\n')
		confluenceURL = strings.TrimSpace(confluenceURL)

		fmt.Print("Confluence Token: ")
		confluenceToken, _ := reader.ReadString('\n')
		confluenceToken = strings.TrimSpace(confluenceToken)

		fmt.Print("Jira URL (e.g. https://your-domain.atlassian.net): ")
		jiraURL, _ := reader.ReadString('\n')
		jiraURL = strings.TrimSpace(jiraURL)

		fmt.Print("Jira Token: ")
		jiraToken, _ := reader.ReadString('\n')
		jiraToken = strings.TrimSpace(jiraToken)

		fmt.Print("Git Username: ")
		gitUser, _ := reader.ReadString('\n')
		gitUser = strings.TrimSpace(gitUser)

		fmt.Print("Git Token: ")
		gitToken, _ := reader.ReadString('\n')
		gitToken = strings.TrimSpace(gitToken)
		
		fmt.Print("GitLab Server URL (e.g. https://gitlab.com): ")
		gitServerURL, _ := reader.ReadString('\n')
		gitServerURL = strings.TrimSpace(gitServerURL)

		fmt.Print("GitLab Project Path (e.g. my-org/my-project): ")
		gitProjectPath, _ := reader.ReadString('\n')
		gitProjectPath = strings.TrimSpace(gitProjectPath)

		template := config.DefaultConfigTemplate
		if confluenceURL != "" {
			template = strings.Replace(template, `"PLACEHOLDER_CONFLUENCE_URL"`, fmt.Sprintf(`"%s"`, confluenceURL), 1)
		}
		if confluenceToken != "" {
			template = strings.Replace(template, `"PLACEHOLDER_CONFLUENCE_TOKEN"`, fmt.Sprintf(`"%s"`, confluenceToken), 1)
		}
		if jiraURL != "" {
			template = strings.Replace(template, `"PLACEHOLDER_JIRA_URL"`, fmt.Sprintf(`"%s"`, jiraURL), 1)
		}
		if jiraToken != "" {
			template = strings.Replace(template, `"PLACEHOLDER_JIRA_TOKEN"`, fmt.Sprintf(`"%s"`, jiraToken), 1)
		}
		if gitUser != "" {
			template = strings.Replace(template, `"PLACEHOLDER_GIT_USERNAME"`, fmt.Sprintf(`"%s"`, gitUser), 1)
		}
		if gitToken != "" {
			template = strings.Replace(template, `"PLACEHOLDER_GIT_TOKEN"`, fmt.Sprintf(`"%s"`, gitToken), 1)
		}
		if gitServerURL != "" {
			template = strings.Replace(template, `"PLACEHOLDER_GITLAB_URL"`, fmt.Sprintf(`"%s"`, gitServerURL), 1)
		}
		if gitProjectPath != "" {
			template = strings.Replace(template, `"PLACEHOLDER_GITLAB_PROJECT_PATH"`, fmt.Sprintf(`"%s"`, gitProjectPath), 1)
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
