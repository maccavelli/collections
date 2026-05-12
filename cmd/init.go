// Package cmd implements the Cobra command tree for the MagicDev MCP server.
package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"
	"mcp-server-magicdev/internal/config"
	"mcp-server-magicdev/internal/embedded"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate the default magicdev.yaml configuration file and exit",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Initializing MagicDev Configuration...")

		created, err := config.EnsureConfig()
		if err != nil {
			return fmt.Errorf("failed to ensure config: %w", err)
		}

		// Extract embedded standards to the OS cache directory.
		cacheDir, err := os.UserCacheDir()
		if err == nil {
			basePath := filepath.Join(cacheDir, "mcp-server-magicdev", "standards")

			// Iterate over each stack's embedded standards directory and write all files.
			for _, stack := range []string{"node", "dotnet"} {
				stackDir := filepath.Join(basePath, stack)
				if err := os.MkdirAll(stackDir, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: failed to create standards dir %s: %v\n", stackDir, err)
					continue
				}

				entries, err := fs.ReadDir(embedded.FS, path.Join("standards", stack))
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: failed to read embedded standards/%s: %v\n", stack, err)
					continue
				}

				for _, entry := range entries {
					if entry.IsDir() {
						continue
					}
					embeddedPath := path.Join("standards", stack, entry.Name())
					b, err := embedded.FS.ReadFile(embeddedPath)
					if err != nil {
						fmt.Fprintf(os.Stderr, "WARNING: failed to read embedded file %s: %v\n", embeddedPath, err)
						continue
					}
					destPath := filepath.Join(stackDir, entry.Name())
					if err := os.WriteFile(destPath, b, 0644); err != nil {
						fmt.Fprintf(os.Stderr, "WARNING: failed to write %s: %v\n", destPath, err)
					}
				}
			}

			// Also extract .gitignore templates from the templates directory.
			for _, tpl := range []struct{ src, stack string }{
				{"templates/Node.gitignore", "node"},
				{"templates/Dotnet.gitignore", "dotnet"},
			} {
				stackDir := filepath.Join(basePath, tpl.stack)
				_ = os.MkdirAll(stackDir, 0755)
				if b, err := embedded.FS.ReadFile(tpl.src); err == nil {
					_ = os.WriteFile(filepath.Join(stackDir, ".gitignore"), b, 0644)
				}
			}
		}

		if !created {
			fmt.Println("Configuration file already exists.")
		} else {
			path, _ := config.ConfigPath()
			fmt.Printf("Default configuration successfully generated at %s\n", path)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
