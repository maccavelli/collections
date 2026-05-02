package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config represents the application configuration.
type Config struct {
	AtlassianURL   string `json:"atlassian_url" mapstructure:"atlassian_url"`
	AtlassianToken string `json:"atlassian_token" mapstructure:"atlassian_token"`
	GitLabToken    string `json:"gitlab_token" mapstructure:"gitlab_token"`
	SSHPrivateKey  string `json:"ssh_private_key" mapstructure:"ssh_private_key"`
}

// ConfigPath returns the absolute path to the config.enc file.
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	magicDir := filepath.Join(dir, "magicdev")
	if err := os.MkdirAll(magicDir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(magicDir, "config.enc"), nil
}

// SaveEncrypted saves the configuration to disk, encrypted with the hardware key.
func SaveEncrypted(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	encrypted, err := Encrypt(string(data))
	if err != nil {
		return err
	}

	return os.WriteFile(path, []byte(encrypted), 0600)
}

// LoadConfig reads the encrypted config file, decrypts it, and loads it into Viper.
func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	encrypted, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	decrypted, err := Decrypt(string(encrypted))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal([]byte(decrypted), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse decrypted config: %w", err)
	}

	// Bind to Viper for global access
	viper.Set("atlassian_url", cfg.AtlassianURL)
	viper.Set("atlassian_token", cfg.AtlassianToken)
	viper.Set("gitlab_token", cfg.GitLabToken)
	viper.Set("ssh_private_key", cfg.SSHPrivateKey)

	return &cfg, nil
}
