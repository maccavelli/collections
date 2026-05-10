package cmd

import (
	"testing"
)

func TestPruneCmd(t *testing.T) {
	err := pruneCmd.RunE(pruneCmd, []string{"-1"})
	if err == nil {
		t.Errorf("Expected error for invalid days")
	}

	err = pruneCmd.RunE(pruneCmd, []string{"abc"})
	if err == nil {
		t.Errorf("Expected error for non-integer days")
	}
}

func TestPruneSubCmds(t *testing.T) {
	for _, sub := range pruneCmd.Commands() {
		err := sub.RunE(sub, []string{"-1"})
		if err == nil {
			t.Errorf("Expected error for invalid days in subcmd")
		}

		err = sub.RunE(sub, []string{"abc"})
		if err == nil {
			t.Errorf("Expected error for non-integer days in subcmd")
		}
	}
}
