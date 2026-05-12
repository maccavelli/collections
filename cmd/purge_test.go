package cmd

import (
	"os"
	"testing"
	"github.com/spf13/viper"
	"mcp-server-magicdev/internal/db"
)

func TestPurgeCommands(t *testing.T) {
	tmpDB, _ := os.CreateTemp("", "testdb-*.db")
	defer os.Remove(tmpDB.Name())
	viper.Set("server.db_path", tmpDB.Name())

	tests := []struct {
		name    string
		command []string
		input   string
	}{
		{"PurgeSessions", []string{"purge", "sessions"}, "y\n"},
		{"PurgeBaselines", []string{"purge", "baselines"}, "y\n"},
		{"PurgeChaos", []string{"purge", "chaos"}, "y\n"},
		{"PurgeSessionsAbort", []string{"purge", "sessions"}, "n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, _ := db.InitStore()
			// Insert dummy data so it can be purged
			if tt.name == "PurgeSessions" {
				store.SaveSession(db.NewSessionState("dummy"))
			}
			store.Close()

			r, w, _ := os.Pipe()
			oldStdin := os.Stdin
			os.Stdin = r
			defer func() { os.Stdin = oldStdin }()

			go func() {
				w.Write([]byte(tt.input))
				w.Close()
			}()

			cmd := rootCmd
			cmd.SetArgs(tt.command)
			err := cmd.Execute()
			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}
