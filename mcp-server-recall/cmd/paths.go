package cmd

import (
	"os"
	"path/filepath"

	"mcp-server-recall/internal/config"
)

// configDirPath returns the OS-idempotent config directory for the application.
//
//	Linux:   ~/.config/mcp-server-recall/
//	macOS:   ~/Library/Application Support/mcp-server-recall/
//	Windows: %AppData%\mcp-server-recall\
func configDirPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	return filepath.Join(configDir, config.Name)
}

// configFilePath returns the full path to the recall.yaml configuration file.
func configFilePath() string {
	return filepath.Join(configDirPath(), "recall.yaml")
}
