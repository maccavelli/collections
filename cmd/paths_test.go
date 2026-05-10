package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPaths(t *testing.T) {
	os.Setenv("XDG_CONFIG_HOME", "/tmp/config")
	defer os.Unsetenv("XDG_CONFIG_HOME")

	dir := configDirPath()
	if dir != "/tmp/config/mcp-server-recall" {
		t.Errorf("expected /tmp/config/mcp-server-recall, got %s", dir)
	}

	path := configFilePath()
	expected := filepath.Join("/tmp/config/mcp-server-recall", "recall.yaml")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}
