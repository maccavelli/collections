//go:build !windows

package ui

// EnableVirtualTerminalProcessing is a no-op on non-Windows platforms.
func EnableVirtualTerminalProcessing() error {
	return nil
}
