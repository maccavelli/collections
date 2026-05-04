package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigSaveLoad(t *testing.T) {
	// Use a temporary directory for config
	tmpDir := t.TempDir()
	
	// Set XDG_CONFIG_HOME to change os.UserConfigDir() on Unix
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	// Fallback for Windows if needed
	t.Setenv("AppData", tmpDir)

	cfg := &Config{
		AtlassianURL:   "https://test.atlassian.net",
		AtlassianToken: "test-token",
		GitLabToken:    "glpat-123",
		SSHPrivateKey:  "ssh-key-data",
	}

	err := SaveEncrypted(cfg)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify file exists
	path, _ := ConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Config file was not created at %s", path)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if loaded.AtlassianURL != cfg.AtlassianURL {
		t.Errorf("Expected URL %q, got %q", cfg.AtlassianURL, loaded.AtlassianURL)
	}
	if loaded.AtlassianToken != cfg.AtlassianToken {
		t.Errorf("Expected Token %q, got %q", cfg.AtlassianToken, loaded.AtlassianToken)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error loading missing config file")
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	
	path, _ := ConfigPath()
	os.MkdirAll(filepath.Dir(path), 0700)
	
	// Write encrypted but non-JSON data
	encrypted, _ := Encrypt("not-valid-json")
	os.WriteFile(path, []byte(encrypted), 0600)

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error parsing invalid JSON config")
	}
}
