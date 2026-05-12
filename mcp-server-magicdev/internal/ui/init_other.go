//go:build !windows

// Package ui provides functionality for the ui subsystem.
package ui

// EnableVirtualTerminalProcessing is a no-op on non-Windows platforms.
func EnableVirtualTerminalProcessing() error {
	return nil
}
