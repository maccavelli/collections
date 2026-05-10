package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestConfig_Defaults(t *testing.T) {
	viper.Reset()
	t.Setenv("HOME", t.TempDir())
	c := New("test-server")

	_ = c.DedupThreshold()
	b := c.BatchSettings()
	if b.MaxBatchSize <= 0 {
		t.Errorf("expected valid batch size")
	}

	_ = c.ExportDir()
	_ = c.EncryptionKey()
	_ = c.HarvestDisableDrift()
}

func TestConfig_LegacyPortMigration(t *testing.T) {
	viper.Reset()

	// Write a stale recall.yaml with the legacy apiport: 7000
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, Name)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	staleConfig := "apiport: 7000\n"
	if err := os.WriteFile(filepath.Join(configDir, "recall.yaml"), []byte(staleConfig), 0600); err != nil {
		t.Fatalf("failed to write stale config: %v", err)
	}

	// Point viper at the temp config directory.
	// On Linux, UserConfigDir() uses XDG_CONFIG_HOME.
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("HOME", tmpDir)

	c := New("test-migration")
	if c.APIPort() != 18001 {
		t.Errorf("expected apiPort to be migrated to 18001, got %d", c.APIPort())
	}
}

func TestConfig_EnvVarIgnored(t *testing.T) {
	viper.Reset()

	// Set the deprecated env var to a non-default value.
	t.Setenv("MCP_RECALL_API_PORT", "9999")
	t.Setenv("MCP_RECALL_APIPORT", "8888")
	t.Setenv("HOME", t.TempDir())

	c := New("test-envignored")

	// apiPort must come from the default (18001), NOT the env var.
	if c.APIPort() != 18001 {
		t.Errorf("expected apiPort default 18001 (env var should be ignored), got %d", c.APIPort())
	}
}
