package cmd

import (
	"testing"
)

func TestExecute(t *testing.T) {
	rootCmd.SetArgs([]string{"--help"})
	if err := Execute(); err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}
