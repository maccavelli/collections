package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	ActiveProvider string                    `json:"active_provider"`
	Providers      map[string]ProviderConfig `json:"providers"`
}

type ProviderConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "prepare-commit-msg", "config.json"), nil
}

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
	return &conf, nil
}

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

func migrateConfig(data []byte, newPath string) (*Config, error) {
	var conf Config
	if err := json.Unmarshal(data, &conf); err != nil {
		return &Config{Providers: make(map[string]ProviderConfig)}, nil // Just start fresh if corrupt
	}
	if conf.Providers == nil {
		conf.Providers = make(map[string]ProviderConfig)
	}

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
