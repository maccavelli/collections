package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"mcp-server-recall/internal/config"
)

func TestConfigureCommand_Sandboxed(t *testing.T) {
	// 1. Enforce strict sandboxing of XDG_CONFIG_HOME
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	// Inject isolated config container successfully to bypass existingKey checks safely
	Cfg = config.New("test-sandboxed")

	// 2. Silence test output noise strictly
	originalStderr := os.Stderr
	os.Stderr = os.NewFile(0, os.DevNull)
	defer func() { os.Stderr = originalStderr }()

	// 3. Pre-create the config file via init (required by the configure guard)
	if err := initCmd.RunE(initCmd, []string{}); err != nil {
		t.Fatalf("initCmd pre-setup failed: %v", err)
	}

	// 4. Mock os.Stdin to securely simulate the user typing 'n\n'
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	defer r.Close()

	originalStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = originalStdin }()

	// Write 'n' (No encryption) asynchronously
	go func() {
		defer w.Close()
		io.WriteString(w, "n\n")
	}()

	// 5. Manually execute the cobra command purely inside memory
	err = configureCmd.RunE(configureCmd, []string{})
	if err != nil {
		t.Fatalf("configureCmd failed natively: %v", err)
	}

	// 6. Assert the artifact correctly spawned inside our safe tempdir (XDG_CONFIG_HOME)
	// and conclusively DID NOT deploy off-network laterally
	expectedPath := filepath.Join(tempDir, config.Name, "recall.yaml")

	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("Configuration artifact was NOT written to the sandboxed path: %s", expectedPath)
	}

	// 7. Assert standard integrity
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read sandboxed config structurally: %v", err)
	}

	if !bytes.Contains(content, []byte("encryptionkey: \"\"")) && !bytes.Contains(content, []byte("encryptionkey: \n")) && !bytes.Contains(content, []byte("encryptionkey:  ")) {
		t.Errorf("Sanbox configuration artifact did not contain an explicitly blank encryptionkey entry")
	}
}

func TestConfigureCommand_RequiresInit(t *testing.T) {
	// Use a clean temp dir with no existing config
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	Cfg = config.New("test-requires-init")

	// Silence stderr
	origStderr := os.Stderr
	os.Stderr = os.NewFile(0, os.DevNull)
	defer func() { os.Stderr = origStderr }()

	// Running configure without init should fail
	err := configureCmd.RunE(configureCmd, []string{})
	if err == nil {
		t.Fatal("configureCmd should fail when no config file exists")
	}

	if !bytes.Contains([]byte(err.Error()), []byte("init")) {
		t.Errorf("error message should mention 'init', got: %s", err.Error())
	}
}
