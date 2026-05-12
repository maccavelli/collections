// Package config handles magicdev.yaml loading, default template generation,
// and fsnotify hot-reloading for the MagicDev pipeline.
package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

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

  # Disable the Confluence integration entirely.
  # When true, the pipeline bypasses live Confluence API calls during Phase 1
  # connectivity checks and downstream documentation generation steps.
  disable: false

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

  # Disable the Jira integration entirely.
  disable: false
  
  # The custom field ID used for estimating Story Points in your Jira instance.
  # This varies per Jira workspace. You can find it in your Jira field settings.
  # If left empty, story points will NOT be set on created issues.
  story_points_field: ""

# ------------------------------------------------------------------------------
# GitLab Integration
# ------------------------------------------------------------------------------
gitlab:
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

  # Disable the GitLab integration entirely.
  disable: false

# ------------------------------------------------------------------------------
# GitHub Integration
# ------------------------------------------------------------------------------
github:
  # API tokens are managed exclusively via BuntDB vault.
  # Run: mcp-server-magicdev token reconfigure

  # The repository path where artifacts will be pushed.
  # Example: "owner/repo"
  repository: "PLACEHOLDER_GITHUB_REPO"
  
  # The default target branch for pushing generated artifacts if not specified.
  default_branch: "main"

  # Disable the GitHub integration entirely.
  disable: false

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
    # Filesystem path to standard templates (e.g. .gitignore) injected automatically
    path: "{CACHE_DIR}/mcp-server-magicdev/standards/node"
    # Max allowed files constraint for the agent (0 = unlimited)
    total_files: 0
    # Max allowed directory depth constraint for the agent (0 = unlimited)
    max_directory_depth: 0
    urls:
      # --- Embedded Standards (local, offline-available, user-editable) ---
      # [embedded, architecture] Node.js Architecture & Design Standards
      - "{CACHE_DIR}/mcp-server-magicdev/standards/node/architecture_and_design.md"
      # [embedded, conventions] TypeScript & Node.js Idioms
      - "{CACHE_DIR}/mcp-server-magicdev/standards/node/typescript_idioms_and_conventions.md"
      # [embedded, database] Database, Persistence & Caching Standards
      - "{CACHE_DIR}/mcp-server-magicdev/standards/node/database_and_caching.md"
      # [embedded, container] Docker, Kubernetes & Containerization Standards
      - "{CACHE_DIR}/mcp-server-magicdev/standards/node/containerization_and_kubernetes.md"
      # --- Remote Standards (fetched and cached on first sync) ---
      # [runtime, lifecycle] Node.js Release Schedule & LTS
      - "https://raw.githubusercontent.com/nodejs/Release/main/README.md"
      # [architecture, security, testing, production, error-handling] Node.js Best Practices
      - "https://raw.githubusercontent.com/goldbergyoni/nodebestpractices/master/README.md"
      # [container] Docker Best Practices for Node.js
      - "https://raw.githubusercontent.com/nodejs/docker-node/main/docs/BestPractices.md"
      # [container, runtime] Docker Node.js Setup Guide
      - "https://raw.githubusercontent.com/nodejs/docker-node/main/README.md"
  dotnet:
    # Filesystem path to standard templates (e.g. .gitignore) injected automatically
    path: "{CACHE_DIR}/mcp-server-magicdev/standards/dotnet"
    # Max allowed files constraint for the agent (0 = unlimited)
    total_files: 0
    # Max allowed directory depth constraint for the agent (0 = unlimited)
    max_directory_depth: 0
    urls:
      # --- Embedded Standards (local, offline-available, user-editable) ---
      # [embedded, architecture] .NET Architecture & Design Standards
      - "{CACHE_DIR}/mcp-server-magicdev/standards/dotnet/architecture_and_design.md"
      # [embedded, conventions] C# 12 & .NET 8 Idioms
      - "{CACHE_DIR}/mcp-server-magicdev/standards/dotnet/csharp_idioms_and_conventions.md"
      # [embedded, database] Database, Persistence & Caching Standards
      - "{CACHE_DIR}/mcp-server-magicdev/standards/dotnet/database_and_caching.md"
      # [embedded, container] Docker, Kubernetes & Containerization Standards
      - "{CACHE_DIR}/mcp-server-magicdev/standards/dotnet/containerization_and_kubernetes.md"
      # --- Remote Standards (fetched and cached on first sync) ---
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
  # The LLM provider to use for the Intelligence Engine.
  # Valid values: "gemini", "openai", "claude"
  # This value can be hot-reloaded at runtime.
  provider: ""

  # The chosen model used by the Intelligence Engine during requirement analysis.
  # This value can be hot-reloaded at runtime.
  # Examples: "gemini-2.5-flash", "gpt-4o", "claude-3-5-sonnet-latest"
  model: ""

  # Set to true to explicitly bypass the LLM Intelligence Engine.
  # When true, MagicDev will gracefully fall back to deterministic synthesis.
  # This value can be hot-reloaded at runtime.
  disable: false

  # NOTE: The LLM API token is stored securely in the BuntDB vault,
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
		template := DefaultConfigTemplate
		
		// Replace placeholder with dynamic cache directory in an OS idempotent manner
		cacheDir, _ := os.UserCacheDir()
		// Let's actually use strings.ReplaceAll properly
		if cacheDir != "" {
			template = strings.ReplaceAll(template, "{CACHE_DIR}", filepath.ToSlash(cacheDir))
		}

		if err := os.WriteFile(path, []byte(template), 0600); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// LoadConfig reads the magicdev.yaml config file and initializes the fsnotify watcher.
// The watcher is always initialized even if the initial config read fails, allowing
// the server to recover when the user corrects the YAML file.
func LoadConfig() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	// Attempt initial config read. Log but do not return on error —
	// we still need to set up the watcher so the server can recover
	// when the user fixes a broken config.
	if err := viper.ReadInConfig(); err != nil {
		slog.Error("initial config read failed (will watch for corrections)",
			"error", err,
			"file", path,
		)
	}

	// Setup fsnotify hot reloading — always, regardless of initial parse result.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("failed to create fsnotify watcher, hot-reload disabled", "error", err)
		return nil // Non-fatal
	}

	go func() {
		// Debounce timer prevents reading a partially-written file when
		// editors (or tooling) perform multiple rapid sequential writes.
		var debounce *time.Timer

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Handle Write, Create, and Rename events. Rename covers
				// atomic-save patterns (write-to-temp + rename) used by
				// most editors and programmatic tooling.
				isRelevant := event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0

				if isRelevant && filepath.Base(event.Name) == "magicdev.yaml" {
					// Reset debounce timer — coalesce rapid writes into a single reload.
					if debounce != nil {
						debounce.Stop()
					}
					debounce = time.AfterFunc(250*time.Millisecond, func() {
						slog.Info("config file changed, reloading...", "file", event.Name, "op", event.Op.String())
						if err := viper.ReadInConfig(); err != nil {
							slog.Error("failed to hot-reload config (will retry on next change)",
								"error", err,
								"file", event.Name,
							)
						} else {
							// Hot-reload log level from updated config.
							logging.SetLevel(viper.GetString("server.log_level"))
							slog.Info("config hot-reloaded successfully",
								"level", viper.GetString("server.log_level"),
							)

							// Execute any registered hot-reload hooks
							for _, hook := range OnConfigReload {
								hook()
							}
						}
					})
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
