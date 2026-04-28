//go:build !windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/shirou/gopsutil/v4/process"
	"mcp-server-magictools/internal/client"
)

// setupZombieReaper collects zombie child processes via SIGCHLD.
func setupZombieReaper(ctx context.Context) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGCHLD)
	defer signal.Stop(sigs)

	for {
		select {
		case <-ctx.Done():
			return
		case <-sigs:
			for {
				var wstatus syscall.WaitStatus
				pid, err := syscall.Wait4(-1, &wstatus, syscall.WNOHANG, nil)
				if err != nil || pid <= 0 {
					break
				}
				slog.Debug("reaper: child process collected", "pid", pid, "exit_status", wstatus.ExitStatus())
			}
		}
	}
}

// enforceResourceLimits ensures the file descriptor limit is at least 4096.
func enforceResourceLimits() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		slog.Warn("resource: failed to get ulimit", "error", err)
		return
	}

	const minLimit = 4096
	if rLimit.Cur < minLimit {
		slog.Info("resource: increasing open files limit", "old", rLimit.Cur, "new", minLimit)
		rLimit.Cur = minLimit
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
			slog.Error("resource: failed to set ulimit", "error", err)
		}
	}
}

// InitialProcessSweep kills orphaned sub-server processes from previous runs.
func InitialProcessSweep() {
	pids, err := process.Pids()
	if err != nil {
		return
	}

	myPid := int32(os.Getpid())
	myPpid := int32(os.Getppid())

	killed := 0
	for _, pid := range pids {
		if pid == myPid || pid == myPpid || pid <= 1 {
			continue
		}

		p, err := process.NewProcess(pid)
		if err != nil {
			continue
		}

		env, err := p.Environ()
		if err != nil {
			continue
		}

		isOrphan := false
		for _, e := range env {
			if e == client.EnvManaged+"="+client.EnvManagedValue || strings.HasPrefix(e, "MAGIC_TOOLS_PEER_ID=") {
				isOrphan = true
				break
			}
		}

		if isOrphan {
			fmt.Fprintf(os.Stderr, "InitialSweep: reaping orphan %d\n", pid)
			if err := syscall.Kill(-int(pid), syscall.SIGKILL); err != nil {
				slog.Debug("failed to kill process group", "pid", pid, "error", err)
			}
			if err := syscall.Kill(int(pid), syscall.SIGKILL); err != nil {
				slog.Debug("failed to kill process", "pid", pid, "error", err)
			}
			killed++
		}
	}
	if killed > 0 {
		fmt.Fprintf(os.Stderr, "InitialSweep: total orphaned processes reaped: %d\n", killed)
	}
}
