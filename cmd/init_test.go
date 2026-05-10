package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"mcp-server-recall/internal/config"
)

func TestInitCommand_FirstRun(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	Cfg = config.New("test-init-firstrun")

	// Silence stderr
	origStderr := os.Stderr
	os.Stderr = os.NewFile(0, os.DevNull)
	defer func() { os.Stderr = origStderr }()

	if err := initCmd.RunE(initCmd, []string{}); err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	expectedPath := filepath.Join(tempDir, config.Name, "recall.yaml")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("Configuration was NOT created at: %s", expectedPath)
	}

	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	if !bytes.Contains(content, []byte("apiport: 18001")) {
		t.Error("Config missing expected apiport entry")
	}
	if !bytes.Contains(content, []byte("dbpath:")) {
		t.Error("Config missing expected dbpath entry")
	}
}

func TestInitCommand_OverwriteNo(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	Cfg = config.New("test-init-overwrite-no")

	// Silence stderr
	origStderr := os.Stderr
	os.Stderr = os.NewFile(0, os.DevNull)
	defer func() { os.Stderr = origStderr }()

	// Create initial config
	if err := initCmd.RunE(initCmd, []string{}); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	configPath := filepath.Join(tempDir, config.Name, "recall.yaml")
	originalContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read original config: %v", err)
	}

	// Mock stdin: answer 'n' to overwrite prompt
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	defer r.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		defer w.Close()
		io.WriteString(w, "n\n")
	}()

	if err := initCmd.RunE(initCmd, []string{}); err != nil {
		t.Fatalf("second init failed: %v", err)
	}

	// Verify file was NOT overwritten
	afterContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config after no-overwrite: %v", err)
	}

	if !bytes.Equal(originalContent, afterContent) {
		t.Error("Config file was modified despite answering 'n' to overwrite")
	}
}

func TestInitCommand_OverwriteYes(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	Cfg = config.New("test-init-overwrite-yes")

	// Silence stderr
	origStderr := os.Stderr
	os.Stderr = os.NewFile(0, os.DevNull)
	defer func() { os.Stderr = origStderr }()

	// Create initial config, then modify it
	if err := initCmd.RunE(initCmd, []string{}); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	configPath := filepath.Join(tempDir, config.Name, "recall.yaml")
	if err := os.WriteFile(configPath, []byte("modified: true\n"), 0600); err != nil {
		t.Fatalf("Failed to modify config: %v", err)
	}

	// Mock stdin: answer 'y' to overwrite prompt
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	defer r.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		defer w.Close()
		io.WriteString(w, "y\n")
	}()

	if err := initCmd.RunE(initCmd, []string{}); err != nil {
		t.Fatalf("overwrite init failed: %v", err)
	}

	// Verify file was overwritten with the template
	afterContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config after overwrite: %v", err)
	}

	if bytes.Contains(afterContent, []byte("modified: true")) {
		t.Error("Config still contains modified content after answering 'y' to overwrite")
	}
	if !bytes.Contains(afterContent, []byte("apiport: 18001")) {
		t.Error("Config missing expected apiport after overwrite")
	}
}
