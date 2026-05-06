package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func setupTestConfig(t *testing.T) string {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, ".config"))
	return tempDir
}

func TestConfigPath(t *testing.T) {
	setupTestConfig(t)
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath failed: %v", err)
	}
	if filepath.Base(path) != "magicdev.yaml" {
		t.Errorf("Expected filename magicdev.yaml, got %s", filepath.Base(path))
	}
}

func TestEnsureConfig(t *testing.T) {
	setupTestConfig(t)

	// 1. Should create on first run
	created, err := EnsureConfig()
	if err != nil {
		t.Fatalf("EnsureConfig failed: %v", err)
	}
	if !created {
		t.Error("Expected EnsureConfig to return true when creating new file")
	}

	path, _ := ConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("magicdev.yaml was not created")
	}

	// 2. Should skip on second run
	created, err = EnsureConfig()
	if err != nil {
		t.Fatalf("EnsureConfig failed on second run: %v", err)
	}
	if created {
		t.Error("Expected EnsureConfig to return false when file already exists")
	}
}

func TestLoadConfig(t *testing.T) {
	setupTestConfig(t)
	EnsureConfig()

	err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify viper parsed the default template
	val := viper.GetString("agent.default_stack")
	if val != ".NET" {
		t.Errorf("Expected default_stack '.NET', got %q", val)
	}
}

func TestUpdateConfigKey(t *testing.T) {
	setupTestConfig(t)
	EnsureConfig()

	// Update existing key
	err := UpdateConfigKey("agent.default_stack", "Go")
	if err != nil {
		t.Fatalf("UpdateConfigKey failed: %v", err)
	}

	// Reload config and verify
	LoadConfig()
	val := viper.GetString("agent.default_stack")
	if val != "Go" {
		t.Errorf("Expected updated key to be 'Go', got %q", val)
	}

	// Update nested key
	err = UpdateConfigKey("confluence.space", "NEWSPACE")
	if err != nil {
		t.Fatalf("UpdateConfigKey failed for nested key: %v", err)
	}

	LoadConfig()
	if viper.GetString("confluence.space") != "NEWSPACE" {
		t.Errorf("Failed to update nested key")
	}

	// Try updating non-existent key
	err = UpdateConfigKey("does.not.exist", "value")
	if err == nil {
		t.Error("Expected error when updating non-existent key")
	}
}
