//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
	"mcp-server-magictools/internal/client"
)

// setupZombieReaper is a no-op on Windows since process groups inherently track lifecycles avoiding zombies efficiently.
func setupZombieReaper(ctx context.Context) {}

// enforceResourceLimits is a no-op on Windows since file descriptor scaling is inherently managed by the kernel limits implicitly.
func enforceResourceLimits() {}

// InitialProcessSweep targets taskkill against matching processes securely over Win32 execution targets natively avoiding POSIX group kills perfectly.
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
			fmt.Fprintf(os.Stderr, "InitialSweep: reaping orphaned Windows task %d\n", pid)
			killCmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(int(pid)))
			if err := killCmd.Run(); err != nil {
				slog.Debug("failed to taskkill orphaned process", "pid", pid, "error", err)
			}
			killed++
		}
	}
	if killed > 0 {
		fmt.Fprintf(os.Stderr, "InitialSweep: total orphaned processes reaped: %d\n", killed)
	}
}
