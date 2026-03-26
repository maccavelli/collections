package config

import (
	"os"
	"testing"
)

func TestConfig(t *testing.T) {
	os.Setenv(EnvDBPath, "/tmp/mcp-test")
	defer os.Unsetenv(EnvDBPath)

	cfg := New()
	if cfg.DBPath != "/tmp/mcp-test" {
		t.Errorf("expected DBPath to be /tmp/mcp-test, got %s", cfg.DBPath)
	}

	path := cfg.GetDBPath()
	if path != "/tmp/mcp-test" {
		t.Errorf("GetDBPath failed: %s", path)
	}
}
