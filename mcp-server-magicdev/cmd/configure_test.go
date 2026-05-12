package cmd

import (
	"os"
	"testing"
	"github.com/spf13/viper"
	"mcp-server-magicdev/internal/config"
)

func TestConfigureCmd(t *testing.T) {
	tmpConfigHome, _ := os.MkdirTemp("", "magicdev-config-home-*")
	defer os.RemoveAll(tmpConfigHome)
	t.Setenv("XDG_CONFIG_HOME", tmpConfigHome)

	tmpDB, _ := os.CreateTemp("", "testdb-*.db")
	defer os.Remove(tmpDB.Name())
	viper.Set("server.db_path", tmpDB.Name())
	
	tmpConfig, err := os.CreateTemp(tmpConfigHome, "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpConfig.Name())
	t.Setenv("MAGICDEV_CONFIG", tmpConfig.Name())
	config.EnsureConfig()

	// Mock stdin with pipes to simulate user choices
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	os.Stdin = r

	// Simulate selecting "0. Exit"
	go func() {
		w.Write([]byte("0\n"))
		w.Close()
	}()

	cmd := rootCmd
	cmd.SetArgs([]string{"configure"})
	err = cmd.Execute()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestSetupHelpers(t *testing.T) {
	tmpConfigHome, _ := os.MkdirTemp("", "magicdev-config-home-2-*")
	defer os.RemoveAll(tmpConfigHome)
	t.Setenv("XDG_CONFIG_HOME", tmpConfigHome)
	
	tmpConfig, err := os.CreateTemp(tmpConfigHome, "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpConfig.Name())
	t.Setenv("MAGICDEV_CONFIG", tmpConfig.Name())
	config.EnsureConfig()

	tmpDB, _ := os.CreateTemp("", "testdb-*.db")
	defer os.Remove(tmpDB.Name())
	viper.Set("server.db_path", tmpDB.Name())

	tests := []struct {
		name     string
		input    string
		function func()
	}{
		{
			name:     "SetupJira",
			input:    "test@example.com\nsecret-token\nhttps://jira.example.com\n",
			function: func() {
				cmd := rootCmd
				cmd.SetArgs([]string{"configure"})
				_ = cmd.Execute()
			},
		},
		{
			name:     "SetupConfluence",
			input:    "secret-token\nhttps://confluence.example.com\n",
			function: func() {
				cmd := rootCmd
				cmd.SetArgs([]string{"configure"})
				_ = cmd.Execute()
			},
		},
		{
			name:     "SetupGitlab",
			input:    "testuser\nsecret-token\n",
			function: func() {
				cmd := rootCmd
				cmd.SetArgs([]string{"configure"})
				_ = cmd.Execute()
			},
		},
		{
			name:     "SetupLLM_Cancel",
			input:    "0\n",
			function: func() {
				cmd := rootCmd
				cmd.SetArgs([]string{"configure"})
				_ = cmd.Execute()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w, _ := os.Pipe()
			oldStdin := os.Stdin
			os.Stdin = r
			defer func() { os.Stdin = oldStdin }()
			
			go func() {
				switch tt.name {
				case "SetupJira":
					// Select 1, input data, then select 0
					w.Write([]byte("1\n" + tt.input + "0\n"))
				case "SetupConfluence":
					w.Write([]byte("2\n" + tt.input + "0\n"))
				case "SetupGitlab":
					w.Write([]byte("3\n" + tt.input + "0\n"))
				case "SetupLLM_Cancel":
					w.Write([]byte("4\n" + tt.input + "0\n"))
				}
				w.Close()
			}()

			tt.function()
		})
	}
}
