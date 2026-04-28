//go:build !windows

package cmd

// enableVirtualTerminalProcessing is a structural NO-OP on Unix-based OS deployments.
// POSIX systems natively resolve ANSI escape sequences through Standard Out natively.
func enableVirtualTerminalProcessing() {
	// Intentionally empty.
}
