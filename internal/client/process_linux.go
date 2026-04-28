//go:build linux

package client

import (
	"os/exec"
	"syscall"
	"time"
)

func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return killPIDGroup(cmd.Process.Pid)
}

func killPIDGroup(pid int) error {
	// On Linux, setting Setpgid: true makes the child its own session leader.
	// We send SIGTERM first to allow graceful shutdown without blocking the orchestrator.
	_ = syscall.Kill(-pid, syscall.SIGTERM) //nolint:errcheck // best-effort group kill
	_ = syscall.Kill(pid, syscall.SIGTERM)  //nolint:errcheck // fallback for individual process

	// Spawn a background reaper to enforce SIGKILL if the process ignores SIGTERM
	go func() {
		time.Sleep(2 * time.Second)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}()

	return nil
}
