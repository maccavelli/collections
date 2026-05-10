package cmd

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"os"
	"strings"

	"github.com/spf13/cobra"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Interactive setup to securely generate or update the encryption key",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Guard: configuration file must exist (created by 'init')
		configPath := configFilePath()
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return fmt.Errorf("configuration not found at %s\nRun 'mcp-server-recall init' first", configPath)
		}

		existingKey := Cfg.EncryptionKey()
		if len(existingKey) >= 32 {
			fmt.Fprintf(os.Stderr, "✓ Valid encryption key already mapped in configuration.\n")
			return nil
		}

		fmt.Fprintf(os.Stderr, "Interactive Encryption Key Setup\n")
		fmt.Fprintf(os.Stderr, "================================\n")
		fmt.Fprintf(os.Stderr, "No encryption key detected safely in your configuration boundaries.\n")

		reader := bufio.NewReader(os.Stdin)
		fmt.Fprintf(os.Stderr, "Do you want to enable AES-256 encryption-at-rest? [Y/n]: ")
		encChoice, _ := reader.ReadString('\n') // Error intentionally ignored: interactive prompt accepts partial input
		encChoice = strings.TrimSpace(strings.ToLower(encChoice))

		var input string
		if encChoice == "n" || encChoice == "no" {
			input = ""
		} else {
			fmt.Fprintf(os.Stderr, "Please paste a 32-character key, or press [ENTER] to natively generate one: ")
			var readErr error
			input, readErr = reader.ReadString('\n')
			if readErr != nil && readErr != io.EOF {
				return fmt.Errorf("error reading input: %w", readErr)
			}
			input = strings.TrimSpace(input)

			if input == "" {
				keyBytes := make([]byte, 16)
				if _, err := rand.Read(keyBytes); err != nil {
					return fmt.Errorf("error generating key: %w", err)
				}
				input = hex.EncodeToString(keyBytes)
			} else if len(input) != 32 {
				return fmt.Errorf("provided key must be exactly 32 characters in length (got %d)", len(input))
			}
		}

		// Re-render the full template to preserve all YAML comments
		fullConfig := fmt.Sprintf(FullConfigTemplate, input)
		if err := os.WriteFile(configPath, []byte(fullConfig), 0600); err != nil {
			return fmt.Errorf("failed to write config output: %w", err)
		}

		fmt.Fprintf(os.Stderr, "\nConfiguration Successful!\n")
		fmt.Fprintf(os.Stderr, "Saved locally to: %s\n", configPath)
		if input != "" {
			fmt.Fprintf(os.Stderr, "Your new database encryption key has been safely vaulted stringently offline.\n")
		} else {
			fmt.Fprintf(os.Stderr, "Database configured securely for unencrypted operations.\n")
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(configureCmd)
}
