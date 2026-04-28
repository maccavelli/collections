//go:build windows

package cmd

import (
	"golang.org/x/sys/windows"
	"os"
)

// enableVirtualTerminalProcessing intercepts the Windows OS Stdout handle,
// appending explicitly the ENABLE_VIRTUAL_TERMINAL_PROCESSING constraint.
// This allows libraries like pterm to output raw ANSI color and placement codes safely.
func enableVirtualTerminalProcessing() {
	stdout := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(stdout, &mode); err == nil {
		mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
		_ = windows.SetConsoleMode(stdout, mode)
	}
}
