//go:build !linux && (darwin || freebsd || openbsd || netbsd)

package client

import (
	"os/exec"
	"syscall"
)

func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return killPIDGroup(cmd.Process.Pid)
}

func killPIDGroup(pid int) error {
	// On Unix-like systems, Setpgid: true makes the child its own session leader.
	// To kill the entire group (including its own children), we use -PID.
	_ = syscall.Kill(-pid, syscall.SIGKILL) //nolint:errcheck // best-effort group kill
	_ = syscall.Kill(pid, syscall.SIGKILL)  //nolint:errcheck // fallback for individual process
	return nil
}
