package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the application configuration including the active LLM provider,
// per-provider settings, and global operational constraints.
type Config struct {
	ActiveProvider string                    `json:"active_provider"`
	Providers      map[string]ProviderConfig `json:"providers"`
	// TimeoutSeconds is the maximum duration in seconds for LLM generation.
	TimeoutSeconds int `json:"timeout_seconds"`
	// MaxDiffBytes is the maximum size in bytes for the git diff sent to the LLM.
	MaxDiffBytes int `json:"max_diff_bytes"`
	// RetryCount is the total number of retries before giving up.
	RetryCount int `json:"retry_count"`
	// RetryDelaySeconds is the wait time in seconds between each retry.
	RetryDelaySeconds int `json:"retry_delay_seconds"`
}

// ProviderConfig stores credentials and model selection for a single LLM provider.
type ProviderConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

// GetConfigPath returns the absolute path to the configuration file.
func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "prepare-commit-msg", "config.json"), nil
}

// Load reads the application configuration from the default configuration path.
// It handles migration from legacy paths if necessary and initializes empty provider mappings.
func Load() (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Migration check
			home, _ := os.UserHomeDir()
			oldPath := filepath.Join(home, ".config", "prepare-commit-msg-embedded", "config.json")
			if oldData, err := os.ReadFile(oldPath); err == nil {
				return migrateConfig(oldData, path)
			}
			return &Config{Providers: make(map[string]ProviderConfig)}, nil
		}
		return nil, err
	}

	var conf Config
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, err
	}
	if conf.Providers == nil {
		conf.Providers = make(map[string]ProviderConfig)
	}

	// TIER 3 PERFORMANCE & PORTABILITY: Ensure all supported providers are visible in config.
	// This makes the config self-documenting for end-users.
	for _, p := range []string{"openai", "gemini", "anthropic"} {
		if _, ok := conf.Providers[p]; !ok {
			conf.Providers[p] = ProviderConfig{}
		}
	}

	// Set defaults if missing
	if conf.TimeoutSeconds <= 0 {
		conf.TimeoutSeconds = 120
	}
	if conf.MaxDiffBytes <= 0 {
		conf.MaxDiffBytes = 32000
	}
	if conf.RetryCount <= 0 {
		conf.RetryCount = 3
	}
	if conf.RetryDelaySeconds <= 0 {
		conf.RetryDelaySeconds = 3
	}

	return &conf, nil
}

// Save persists the current configuration to the disk in JSON format.
// It ensures the parent directory exists and sets file permissions to 0600.
func (c *Config) Save() error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// GetActive retrieves the configuration details for the currently active provider.
// It returns an error if no provider is active or if the active provider is missing from the configuration.
func (c *Config) GetActive() (ProviderConfig, error) {
	if c.ActiveProvider == "" {
		return ProviderConfig{}, fmt.Errorf("no active provider configured; please run with --setup")
	}
	pc, ok := c.Providers[c.ActiveProvider]
	if !ok {
		return ProviderConfig{}, fmt.Errorf("active provider %q not found in config", c.ActiveProvider)
	}
	return pc, nil
}

// migrateConfig migrates configuration data from a legacy location to a new path.
// It silently handles failures during migration by starting over with a fresh configuration.
func migrateConfig(data []byte, newPath string) (*Config, error) {
	var conf Config
	if err := json.Unmarshal(data, &conf); err != nil {
		return &Config{Providers: make(map[string]ProviderConfig)}, nil // Just start fresh if corrupt
	}
	if conf.Providers == nil {
		conf.Providers = make(map[string]ProviderConfig)
	}

	// TIER 3: Ensure consistency post-migration
	for _, p := range []string{"openai", "gemini", "anthropic"} {
		if _, ok := conf.Providers[p]; !ok {
			conf.Providers[p] = ProviderConfig{}
		}
	}

	// Update the data slice with the initialized fields before saving
	data, _ = json.MarshalIndent(&conf, "", "  ")

	// Save to new path immediately
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err == nil {
		if err := os.WriteFile(newPath, data, 0600); err == nil {
			// Succeeded migrating file, now try to clean up old dir
			home, _ := os.UserHomeDir()
			oldDir := filepath.Join(home, ".config", "prepare-commit-msg-embedded")
			os.RemoveAll(oldDir)
		}
	}

	return &conf, nil
}
