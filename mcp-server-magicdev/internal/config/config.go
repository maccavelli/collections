// Package config handles magicdev.yaml loading, default template generation,
// and fsnotify hot-reloading for the MagicDev pipeline.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

const DefaultConfigTemplate = `# ==============================================================================
# MagicDev Server Configuration
# ==============================================================================

# Integration: Atlassian (Jira & Confluence)
atlassian:
  # The base URL for your Atlassian Cloud instance.
  # Example: https://your-domain.atlassian.net
  url: "PLACEHOLDER_ATLASSIAN_URL"
  
  # An API token for Atlassian Cloud.
  # Required to create tickets and post documentation.
  token: "PLACEHOLDER_ATLASSIAN_TOKEN"
  
  # Custom field ID for Story Points in your Jira instance (if applicable).
  story_points_field: "customfield_10016"

# Integration: Git (GitHub / GitLab)
git:
  # Your git service username.
  username: "PLACEHOLDER_GIT_USERNAME"
  
  # A personal access token for committing documents to Git over HTTPS.
  token: "PLACEHOLDER_GIT_TOKEN"
  
  # Default target branch for pushing generated artifacts.
  default_branch: "main"

# Core Agent Settings
agent:
  # The default tech stack assumed when analyzing requirements (e.g., .NET, Node)
  default_stack: ".NET"

# Server Diagnostics
server:
  # Set the database persistence path. ":memory:" provides ephemeral storage.
  # Use an absolute path (e.g., "/opt/magicdev/state.db") to persist sessions.
  db_path: ":memory:"
`

// ConfigPath returns the absolute path to the magicdev.yaml file.
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	magicDir := filepath.Join(dir, "magicdev")
	if err := os.MkdirAll(magicDir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(magicDir, "magicdev.yaml"), nil
}

// EnsureConfig checks if the config exists, creating it with the default template if it doesn't.
// It returns true if it had to create a new file.
func EnsureConfig() (bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return false, err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(DefaultConfigTemplate), 0600); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// LoadConfig reads the magicdev.yaml config file and initializes the fsnotify watcher.
func LoadConfig() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Setup fsnotify hot reloading
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("failed to create fsnotify watcher, hot-reload disabled", "error", err)
		return nil // Non-fatal
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// We care about Write or Create events on the magicdev.yaml file
				isWrite := event.Op&fsnotify.Write == fsnotify.Write
				isCreate := event.Op&fsnotify.Create == fsnotify.Create
				
				if (isWrite || isCreate) && filepath.Base(event.Name) == "magicdev.yaml" {
					slog.Info("config file changed, reloading...", "file", event.Name)
					if err := viper.ReadInConfig(); err != nil {
						slog.Error("failed to hot-reload config", "error", err)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Error("fsnotify watcher error", "error", err)
			}
		}
	}()

	// Windows locks files heavily; watch the directory instead. Linux/Mac watch the file directly.
	if runtime.GOOS == "windows" {
		watchDir := filepath.Dir(path)
		if err := watcher.Add(watchDir); err != nil {
			slog.Warn("failed to watch config directory", "dir", watchDir, "error", err)
		}
	} else {
		if err := watcher.Add(path); err != nil {
			slog.Warn("failed to watch config file", "file", path, "error", err)
		}
	}

	return nil
}
