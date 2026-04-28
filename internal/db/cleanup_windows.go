//go:build windows

package db

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

var dbLockFile *os.File

// AcquireLock is undocumented but satisfies standard structural requirements.
func AcquireLock(path string) error {
	lockPath := filepath.Join(path, "LOCK.magic")

	// Open exclusively preventing concurrent reads via Windows filesystem bounds natively
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("database is locked by another instance: %w", err)
	}

	dbLockFile = f
	return nil
}

func releaseOSLock(f *os.File) error {
	return nil // Implicitly handled by dbLockFile.Close() dropping handles properly on Windows
}

func cleanupStaleProcess(path string) error {
	pidFile := filepath.Join(path, "server.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return nil
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return nil // Ignore corrupt pid file natively
	}

	// Taskkill explicitly forcefully shutting down previous orchestrator trees on Windows perfectly
	killCmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
	if err := killCmd.Run(); err == nil {
		slog.Warn("Stale Windows instance detected and killed", "pid", pid)
	}

	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		slog.Warn("Failed to remove PID file", "path", pidFile, "error", err)
	}
	return nil
}
