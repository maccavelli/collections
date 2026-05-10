package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create the default configuration file",
	Long:  "Generates the default recall.yaml configuration file at the OS-standard config directory. If a configuration already exists, prompts before overwriting.",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath := configFilePath()

		// Check if configuration already exists
		if _, err := os.Stat(configPath); err == nil {
			fmt.Fprintf(os.Stderr, "Configuration already exists at %s\n", configPath)
			fmt.Fprintf(os.Stderr, "Overwrite? [y/N]: ")

			reader := bufio.NewReader(os.Stdin)
			choice, _ := reader.ReadString('\n')
			choice = strings.TrimSpace(strings.ToLower(choice))

			if choice != "y" && choice != "yes" {
				fmt.Fprintf(os.Stderr, "Configuration preserved.\n")
				return nil
			}
		}

		// Create the config directory with restrictive permissions
		dirPath := configDirPath()
		if err := os.MkdirAll(dirPath, 0700); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		// Write the default configuration template with an empty encryption key
		fullConfig := fmt.Sprintf(FullConfigTemplate, "")
		if err := os.WriteFile(configPath, []byte(fullConfig), 0600); err != nil {
			return fmt.Errorf("failed to write configuration: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Configuration initialized at: %s\n", configPath)
		fmt.Fprintf(os.Stderr, "Run 'mcp-server-recall configure' to set up encryption.\n")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(initCmd)
}
