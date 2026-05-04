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
	Short: "Configure credentials securely for MagicDev",
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)

		fmt.Println("=== MagicDev Configuration ===")
		fmt.Println("Credentials will be AES-256-GCM encrypted using a hardware-derived key.")
		fmt.Println()

		url, err := promptInput(reader, "Atlassian Site URL (e.g., https://your-domain.atlassian.net)")
		if err != nil {
			return fmt.Errorf("failed to read Atlassian URL: %w", err)
		}

		aToken, err := promptInput(reader, "Atlassian API Token")
		if err != nil {
			return fmt.Errorf("failed to read Atlassian token: %w", err)
		}

		gToken, err := promptInput(reader, "GitLab Personal Access Token")
		if err != nil {
			return fmt.Errorf("failed to read GitLab token: %w", err)
		}

		sshPath, err := promptInput(reader, "SSH Private Key Path (e.g., ~/.ssh/id_rsa)")
		if err != nil {
			return fmt.Errorf("failed to read SSH key path: %w", err)
		}

		cfg := &config.Config{
			AtlassianURL:   url,
			AtlassianToken: aToken,
			GitLabToken:    gToken,
			SSHPrivateKey:  sshPath,
		}

		if err := config.SaveEncrypted(cfg); err != nil {
			return fmt.Errorf("failed to save encrypted configuration: %w", err)
		}

		fmt.Println("\nConfiguration successfully saved and hardware-encrypted.")
		return nil
	},
}

// promptInput displays a label and reads a trimmed line from the reader.
func promptInput(reader *bufio.Reader, label string) (string, error) {
	fmt.Printf("%s: ", label)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func init() {
	rootCmd.AddCommand(configureCmd)
}
