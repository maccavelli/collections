// Package config handles magicdev.yaml loading, default template generation,
// and fsnotify hot-reloading for the MagicDev pipeline.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"mcp-server-magicdev/internal/logging"
)

const DefaultConfigTemplate = `# ==============================================================================
# MagicDev Server Configuration (magicdev.yaml)
#
# This file governs the integrations, behavior, and structural boundaries of the
# MagicDev Socratic Orchestrator. The orchestrator uses fsnotify to hot-reload
# changes made to this file in real-time.
# ==============================================================================

# ------------------------------------------------------------------------------
# Confluence Integration (Documentation)
# ------------------------------------------------------------------------------
confluence:
  # The base URL for your Confluence instance.
  # Example: https://your-domain.atlassian.net/wiki
  url: "PLACEHOLDER_CONFLUENCE_URL"

  # API tokens are managed exclusively via BuntDB vault.
  # Run: mcp-server-magicdev token reconfigure

  # The space key where documentation will be published.
  # Default: "SPACE"
  space: "SPACE"
  
  # An optional parent page ID to nest generated documents underneath.
  # If left unset (""), documents will be published to the root of the space.
  parent_page_id: ""

  # Enables the Confluence mock layer for offline development and testing.
  # When true, the pipeline bypasses live Confluence API calls during Phase 1
  # connectivity checks and downstream documentation generation steps.
  mock: false

# ------------------------------------------------------------------------------
# Jira Integration (Ticketing)
# ------------------------------------------------------------------------------
jira:
  # The user email for Jira authentication.
  email: "PLACEHOLDER_JIRA_EMAIL"

  # The base URL for your Jira instance.
  # Example: https://your-domain.atlassian.net
  url: "PLACEHOLDER_JIRA_URL"

  # API tokens are managed exclusively via BuntDB vault.
  # Run: mcp-server-magicdev token reconfigure
  
  # The project key where issues should be created.
  # Default: "PROJ"
  project: "PROJ"
  
  # An existing issue key to attach documents to.
  # If left unset (""), MagicDev will automatically create a new task.
  issue: ""

  # Enables the Jira mock layer for offline development and testing.
  mock: false
  
  # The custom field ID used for estimating Story Points in your Jira instance.
  # This varies per Jira workspace. You can find it in your Jira field settings.
  # If left empty, story points will NOT be set on created issues.
  story_points_field: ""

# ------------------------------------------------------------------------------
# Version Control Integration (Git)
# ------------------------------------------------------------------------------
# These settings allow MagicDev to commit artifacts and generated code to Git
# using the native HTTPS transport protocol.
git:
  # Your git service username (e.g., your GitHub or GitLab handle).
  username: "PLACEHOLDER_GIT_USERNAME"

  # API tokens are managed exclusively via BuntDB vault.
  # Run: mcp-server-magicdev token reconfigure

  # The base URL for the GitLab server.
  # Example: "https://gitlab.com" or "https://gitlab.internal.corp"
  server_url: "PLACEHOLDER_GITLAB_URL"

  # The namespace/project path in GitLab where artifacts will be pushed.
  # Example: "my-org/my-project"
  project_path: "PLACEHOLDER_GITLAB_PROJECT_PATH"
  
  # The default target branch for pushing generated artifacts if not specified.
  default_branch: "main"

# ------------------------------------------------------------------------------
# Core Agent Behavior Settings
# ------------------------------------------------------------------------------
agent:
  # The default technology stack assumed when analyzing requirements or
  # generating technical blueprints if the user does not specify one.
  # Recommended values: ".NET", "Node", "Go", "Python"
  default_stack: ".NET"

# ------------------------------------------------------------------------------
# OS & Runtime Optimization (Advanced)
# ------------------------------------------------------------------------------
# MagicDev applies these limits directly to the Go runtime at startup to ensure
# stable execution in dynamic CPU-scaling environments (e.g., Kubernetes pods).
runtime:
  # The soft memory limit for the Go garbage collector. As memory pressure 
  # approaches this limit, the GC will aggressively reclaim memory to prevent OOM.
  # Valid formats: "4GB", "512MB", etc.
  gomemlimit: "4GB"
  
  # The maximum number of OS threads that can execute user-level Go code.
  # Keeping this low prevents CPU context-switching thrashing in constrained environments.
  gomaxprocs: "2"

# ------------------------------------------------------------------------------
# Server Diagnostics & Storage
# ------------------------------------------------------------------------------
server:
  # The log level for the MagicDev server output.
  # Valid values: DEBUG, INFO, WARN, ERROR
  # Changes to this value are hot-reloaded via fsnotify.
  # Default: "INFO"
  log_level: "INFO"

  # The absolute path to the BuntDB persistence file (session.db).
  # If left blank (""), MagicDev will automatically use the OS default cache directory:
  # Linux: $HOME/.cache/mcp-server-magicdev/session.db
  # Windows: %LOCALAPPDATA%\mcp-server-magicdev\session.db
  # macOS: $HOME/Library/Caches/mcp-server-magicdev/session.db
  db_path: ""

# ------------------------------------------------------------------------------
# Baseline Architectural Standards
# ------------------------------------------------------------------------------
# These URLs are automatically fetched by the sync engine on startup. The engine
# compresses them with Zstd and caches them in BuntDB for zero-latency retrieval.
# You can add local filesystem paths (e.g., /path/to/my/standards.md) or standard URLs.
standards:
  node:
    # [runtime, lifecycle] Node.js Release Schedule & LTS
    - "https://raw.githubusercontent.com/nodejs/Release/main/README.md"
    # [architecture, security, testing, production, error-handling] Node.js Best Practices
    - "https://raw.githubusercontent.com/goldbergyoni/nodebestpractices/master/README.md"
    # [container] Docker Best Practices for Node.js
    - "https://raw.githubusercontent.com/nodejs/docker-node/main/docs/BestPractices.md"
    # [container, runtime] Docker Node.js Setup Guide
    - "https://raw.githubusercontent.com/nodejs/docker-node/main/README.md"
  dotnet:
    # [runtime, lifecycle] .NET 8.0 Release Notes
    - "https://raw.githubusercontent.com/dotnet/core/main/release-notes/8.0/README.md"
    # [code-style] C# Coding Conventions
    - "https://raw.githubusercontent.com/dotnet/docs/main/docs/csharp/fundamentals/coding-style/coding-conventions.md"
    # [architecture] .NET Design Guidelines
    - "https://raw.githubusercontent.com/dotnet/docs/main/docs/standard/design-guidelines/index.md"
    # [security] Secure Coding Guidelines for .NET
    - "https://raw.githubusercontent.com/dotnet/docs/main/docs/standard/security/secure-coding-guidelines.md"
    # [testing] Unit Testing Best Practices
    - "https://raw.githubusercontent.com/dotnet/docs/main/docs/core/testing/unit-testing-best-practices.md"
    # [container] .NET Docker Samples
    - "https://raw.githubusercontent.com/dotnet/dotnet-docker/main/samples/README.md"
    # [container, runtime] Installing .NET in Docker
    - "https://raw.githubusercontent.com/dotnet/dotnet-docker/main/documentation/scenarios/installing-dotnet.md"

# ------------------------------------------------------------------------------
# Intelligence Engine (LLM)
# ------------------------------------------------------------------------------
llm:
  # The chosen model used by the Intelligence Engine during requirement analysis.
  # This value can be hot-reloaded at runtime.
  # Examples: "gemini-2.5-flash", "gpt-4o", "claude-3-5-sonnet-latest"
  model: ""

  # NOTE: The LLM API token and provider are stored securely in the BuntDB vault,
  # NOT in this configuration file.
  # To set up the LLM, run: mcp-server-magicdev configure
`

// OnConfigReload contains functions to be called whenever fsnotify reloads the configuration.
var OnConfigReload []func()

// ConfigPath returns the absolute path to the magicdev.yaml file.
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	magicDir := filepath.Join(dir, "mcp-server-magicdev")
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
					} else {
						// Hot-reload log level from updated config.
						logging.SetLevel(viper.GetString("server.log_level"))
						slog.Info("log level hot-reloaded", "level", viper.GetString("server.log_level"))
						
						// Execute any registered hot-reload hooks
						for _, hook := range OnConfigReload {
							hook()
						}
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

	// Always watch the directory, not the file. Most editors use atomic
	// save (write-to-temp + rename) which replaces the inode, causing
	// file-level watchers to silently detach.
	watchDir := filepath.Dir(path)
	if err := watcher.Add(watchDir); err != nil {
		slog.Warn("failed to watch config directory", "dir", watchDir, "error", err)
	}

	return nil
}
