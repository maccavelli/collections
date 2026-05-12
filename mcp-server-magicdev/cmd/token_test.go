package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"mcp-server-magicdev/internal/db"
)

func TestTokenListCmd(t *testing.T) {
	tmpDB, _ := os.CreateTemp("", "testdb-*.db")
	defer os.Remove(tmpDB.Name())
	viper.Set("server.db_path", tmpDB.Name())

	store, _ := db.InitStore()
	store.SetSecret("gitlab", "glpat-test-123")
	store.Close()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := rootCmd
	cmd.SetArgs([]string{"token", "list"})
	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !strings.Contains(out, "glpat-test-123") {
		t.Errorf("Expected token output, got %s", out)
	}
}

func TestTokenReconfigureCmd(t *testing.T) {
	tmpDB, _ := os.CreateTemp("", "testdb-*.db")
	defer os.Remove(tmpDB.Name())
	viper.Set("server.db_path", tmpDB.Name())

	os.Setenv("GITLAB_TOKEN", "glpat-env-test")
	defer os.Unsetenv("GITLAB_TOKEN")
	os.Unsetenv("GEMINI_API_KEY")

	// Mock stdin for inputs where env is missing
	r, w, _ := os.Pipe()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		// Provide empty string for missing envs to skip them
		w.Write([]byte("\n\n\n\n\n\n"))
		w.Close()
	}()

	cmd := rootCmd
	cmd.SetArgs([]string{"token", "reconfigure"})
	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	store, _ := db.InitStore()
	val, _ := store.GetSecret("gitlab")
	store.Close()
	if val != "glpat-env-test" {
		t.Errorf("Expected token from env to be stored, got %s", val)
	}
}
