//go:build windows

// Package lifecycle provides functionality for the lifecycle subsystem.
package lifecycle

import (
	"os"

	"golang.org/x/sys/windows"
)

// isParentAlive checks whether the original parent process is still running.
//
// On Windows, os.Getppid() returns the original parent PID even after the
// parent exits (no reparenting to PID 1 like Unix). Instead, we open a
// handle to the parent process using OpenProcess and check if it is still
// accessible. If the process has exited, OpenProcess will fail with
// ERROR_INVALID_PARAMETER.
func isParentAlive(originalPPID int) bool {
	currentPPID := os.Getppid()

	// PID changed — parent was replaced (rare on Windows but defensive).
	if currentPPID != originalPPID {
		return false
	}

	// Attempt to open the parent process with minimal access rights.
	// PROCESS_QUERY_LIMITED_INFORMATION is the least-privilege flag that
	// still validates process existence.
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(originalPPID),
	)
	if err != nil {
		// Process doesn't exist or access denied — treat as dead.
		return false
	}
	windows.CloseHandle(handle)
	return true
}
