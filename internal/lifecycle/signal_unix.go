//go:build !windows

package lifecycle

import (
	"os"
	"syscall"
)

// isParentAlive checks whether the original parent process is still running.
//
// On Linux/macOS, os.Getppid() returns 1 when the parent dies (reparented
// to init/launchd). As a secondary check, we send signal 0 to probe the
// process without delivering a real signal.
func isParentAlive(originalPPID int) bool {
	currentPPID := os.Getppid()

	// Fast path: reparented to init/launchd means parent is gone.
	if currentPPID == 1 {
		return false
	}

	// PID changed to something other than the original — parent was replaced.
	if currentPPID != originalPPID {
		return false
	}

	// Signal 0 probe: checks process existence without side effects.
	proc, err := os.FindProcess(originalPPID)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
