package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_SaveAndLoad(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	conf := &Config{
		ActiveProvider: "test-provider",
		Providers: map[string]ProviderConfig{
			"test-provider": {
				APIKey: "test-key",
				Model:  "test-model",
			},
		},
	}

	if err := conf.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.ActiveProvider != conf.ActiveProvider {
		t.Errorf("expected active provider %q, got %q", conf.ActiveProvider, loaded.ActiveProvider)
	}

	pc, ok := loaded.Providers["test-provider"]
	if !ok || pc.APIKey != "test-key" {
		t.Errorf("expected provider config not found or incorrect")
	}
}

func TestConfig_GetActive(t *testing.T) {
	conf := &Config{
		ActiveProvider: "p1",
		Providers: map[string]ProviderConfig{
			"p1": {APIKey: "k1", Model: "m1"},
		},
	}

	pc, err := conf.GetActive()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if pc.APIKey != "k1" {
		t.Errorf("expected k1, got %s", pc.APIKey)
	}

	conf.ActiveProvider = "non-existent"
	_, err = conf.GetActive()
	if err == nil {
		t.Error("expected error for non-existent provider, got nil")
	}

	conf.ActiveProvider = ""
	_, err = conf.GetActive()
	if err == nil {
		t.Error("expected error for empty active provider, got nil")
	}
}

func TestConfig_Migration(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	oldDir := filepath.Join(tmpHome, ".config", "prepare-commit-msg-embedded")
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldConfigPath := filepath.Join(oldDir, "config.json")
	
	oldConf := Config{
		ActiveProvider: "legacy",
		Providers: map[string]ProviderConfig{
			"legacy": {APIKey: "old-key", Model: "old-model"},
		},
	}
	data, _ := json.Marshal(oldConf)
	if err := os.WriteFile(oldConfigPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Load should trigger migration
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed during migration: %v", err)
	}

	if loaded.ActiveProvider != "legacy" {
		t.Errorf("expected migrated provider 'legacy', got %q", loaded.ActiveProvider)
	}

	// Verify old dir is gone
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("expected old config directory to be removed after migration")
	}

	// Verify new file exists
	newPath, _ := GetConfigPath()
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected new config file to exist at %s", newPath)
	}
}
func TestConfig_UserRepro(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	configDir := filepath.Join(tmpHome, ".config", "prepare-commit-msg")
	os.MkdirAll(configDir, 0755)
	configPath := filepath.Join(configDir, "config.json")

	userJson := `{
  "active_provider": "gemini",
  "providers": {
    "gemini": {
      "api_key": "REDACTED_API_KEY",
      "model": "gemini-2.5-flash-lite"
    },
    "openai": {
      "api_key": "",
      "model": "gpt-4o"
    }
  },
  "timeout_seconds": 120,
  "max_diff_bytes": 32000,
  "retry_count": 3,
  "retry_delay_seconds": 3
}`
	os.WriteFile(configPath, []byte(userJson), 0600)

	// 1. Load existing
	conf, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 2. Perform setup-like update
	provider := "anthropic"
	pc, ok := conf.Providers[provider]
	if !ok {
		pc = ProviderConfig{}
	}
	pc.APIKey = "new-anthropic-key"
	pc.Model = "claude-3-5-haiku-latest"
	conf.ActiveProvider = provider
	conf.Providers[provider] = pc

	// 3. Save
	if err := conf.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 4. Reload and Verify
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if loaded.ActiveProvider != "anthropic" {
		t.Errorf("Expected active provider 'anthropic', got %q", loaded.ActiveProvider)
	}

	if pc, ok := loaded.Providers["anthropic"]; !ok || pc.APIKey != "new-anthropic-key" {
		t.Error("Anthropic provider missing or incorrect after reload")
	}
	if pc, ok := loaded.Providers["gemini"]; !ok || pc.APIKey != "REDACTED_API_KEY" {
		t.Error("Gemini provider lost or incorrect after reload")
	}
}

func TestFallbackModels(t *testing.T) {
	c := Config{
		Providers: map[string]ProviderConfig{
			"openai": {Model: "gpt-4o", FallbackModels: []string{"gpt-4o-mini"}},
		},
	}
	pc := c.Providers["openai"]
	if len(pc.FallbackModels) != 1 || pc.FallbackModels[0] != "gpt-4o-mini" {
		t.Errorf("expected 1 fallback model 'gpt-4o-mini', got %v", pc.FallbackModels)
	}
}

func TestGetConfigPath_Error(t *testing.T) {
	oldFn := userHomeDir
	defer func() { userHomeDir = oldFn }()

	userHomeDir = func() (string, error) {
		return "", fmt.Errorf("mock homedir error")
	}

	_, err := GetConfigPath()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestLoad_Errors(t *testing.T) {
	// 1. File read error other than IsNotExist
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	configDir := filepath.Join(tmpDir, ".config", "prepare-commit-msg")
	os.MkdirAll(configDir, 0755)

	// Create a directory where the file should be, causing ReadFile to fail with IsDir
	configPath := filepath.Join(configDir, "config.json")
	os.Mkdir(configPath, 0755)

	_, err := Load()
	if err == nil {
		t.Error("expected error reading a directory, got nil")
	}
	os.RemoveAll(configPath)

	// 2. Unmarshal error
	badJSON := `{ bad, json }`
	os.WriteFile(configPath, []byte(badJSON), 0644)
	_, err = Load()
	if err == nil {
		t.Error("expected unmarshal error, got nil")
	}
}

func TestSave_Errors(t *testing.T) {
	c := &Config{}

	// mock home dir failure
	oldFn := userHomeDir
	defer func() { userHomeDir = oldFn }()

	userHomeDir = func() (string, error) {
		return "", fmt.Errorf("mock error")
	}
	if err := c.Save(); err == nil {
		t.Error("expected save error due to home dir failure")
	}

	// Restore home dir
	userHomeDir = oldFn

	// Mock failing MkdirAll: we'll create a file with the same name as the intended config directory
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	configDir := filepath.Join(tmpDir, ".config", "prepare-commit-msg")
	os.MkdirAll(filepath.Dir(configDir), 0755)
	os.WriteFile(configDir, []byte("file-not-dir"), 0644)

	if err := c.Save(); err == nil {
		t.Error("expected save error due to mkdir failure")
	}
}

func TestMigrateConfig_CorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	legacyDir := filepath.Join(tmpDir, ".config", "prepare-commit-msg-embedded")
	os.MkdirAll(legacyDir, 0755)
	legacyConfPath := filepath.Join(legacyDir, "config.json")

	badData := []byte(`{ bad }`)
	os.WriteFile(legacyConfPath, badData, 0644)

	// In Load, it will discover this and call migrateConfig.
	c, err := Load()
	if err != nil {
		t.Errorf("migrateConfig should handle corrupt json silently: %v", err)
	}
	if c.Providers == nil {
		t.Error("expected fresh initialized config providers")
	}
}
