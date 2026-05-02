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

		fmt.Print("Atlassian Site URL (e.g., https://your-domain.atlassian.net): ")
		url, _ := reader.ReadString('\n')
		url = strings.TrimSpace(url)

		fmt.Print("Atlassian API Token: ")
		aToken, _ := reader.ReadString('\n')
		aToken = strings.TrimSpace(aToken)

		fmt.Print("GitLab Personal Access Token: ")
		gToken, _ := reader.ReadString('\n')
		gToken = strings.TrimSpace(gToken)

		fmt.Print("SSH Private Key Path (e.g., ~/.ssh/id_rsa): ")
		sshPath, _ := reader.ReadString('\n')
		sshPath = strings.TrimSpace(sshPath)

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

func init() {
	rootCmd.AddCommand(configureCmd)
}
