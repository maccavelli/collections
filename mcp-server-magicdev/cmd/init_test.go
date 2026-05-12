package cmd

import (
	"bytes"
	"testing"
)

func TestInitCmd(t *testing.T) {
	// Temporarily override os.Stdout or just run it to ensure no panics
	out := new(bytes.Buffer)
	initCmd.SetOut(out)
	
	err := initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Errorf("Unexpected error running initCmd: %v", err)
	}
}
