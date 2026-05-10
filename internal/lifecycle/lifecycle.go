// Package lifecycle provides cross-platform single-instance enforcement and
// graceful shutdown management for the MagicSkills MCP server.
//
// It implements a layered defense strategy:
//   - Layer 1: OS-level file lock (flock/LockFileEx) for single-instance enforcement
//   - Layer 2: Parent process watchdog to detect orphaned processes
//   - Layer 3: Graceful shutdown with hard deadline
package lifecycle

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

const (
	// lockFileName is the name of the exclusive lock file.
	lockFileName = "magicskills.lock"

	// lockAcquireTimeout is the maximum time to wait for the lock after
	// killing a stale process.
	lockAcquireTimeout = 5 * time.Second

	// lockRetryInterval is how often we retry TryLock while waiting for the
	// old process to release.
	lockRetryInterval = 250 * time.Millisecond
)

// lockDir returns the directory used for the lock and PID files.
func lockDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	dir := filepath.Join(cacheDir, "mcp-server-magicskills")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create lock directory: %w", err)
	}
	return dir, nil
}

// AcquireLock attempts to acquire an exclusive file lock. If the lock is
// already held by another process, it reads the PID from the lock file,
// kills that process, and retries. Returns the held lock (caller must
// defer Unlock) or an error if the lock cannot be acquired.
func AcquireLock() (*flock.Flock, error) {
	dir, err := lockDir()
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(dir, lockFileName)
	fl := flock.New(lockPath)

	// First attempt: non-blocking try.
	locked, err := fl.TryLock()
	if err != nil {
		return nil, fmt.Errorf("failed to attempt lock: %w", err)
	}
	if locked {
		writePID(lockPath)
		slog.Info("instance lock acquired", "lock", lockPath, "pid", os.Getpid())
		return fl, nil
	}

	// Lock is busy — another instance is running.
	slog.Warn("lock held by another instance, attempting takeover", "lock", lockPath)

	// Read the stale PID and kill it.
	if stalePID, readErr := readPID(lockPath); readErr == nil && stalePID > 0 {
		killProcess(stalePID)
	}

	// Retry with timeout — wait for the old process to release.
	deadline := time.Now().Add(lockAcquireTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(lockRetryInterval)
		locked, err = fl.TryLock()
		if err != nil {
			return nil, fmt.Errorf("failed to retry lock: %w", err)
		}
		if locked {
			writePID(lockPath)
			slog.Info("instance lock acquired after takeover", "lock", lockPath, "pid", os.Getpid())
			return fl, nil
		}
	}

	return nil, fmt.Errorf("failed to acquire instance lock after %s — another instance may be stuck", lockAcquireTimeout)
}

// writePID writes the current process PID to the lock file.
// This is best-effort — failure is logged but not fatal.
func writePID(lockPath string) {
	pidStr := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(lockPath, []byte(pidStr), 0644); err != nil {
		slog.Warn("failed to write PID to lock file", "path", lockPath, "error", err)
	}
}

// readPID reads a PID from the lock file. Returns 0 if the file is
// empty, missing, or contains non-numeric content.
func readPID(lockPath string) (int, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		return 0, fmt.Errorf("lock file empty")
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in lock file: %q", pidStr)
	}
	return pid, nil
}

// killProcess sends a termination signal to the process with the given PID.
// On Unix, this sends SIGTERM first, then SIGKILL after a brief grace period.
// On Windows, os.Process.Kill() calls TerminateProcess().
// This is best-effort — errors are logged but not returned.
func killProcess(pid int) {
	if pid == os.Getpid() {
		// Safety: never kill ourselves.
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		slog.Debug("stale process not found", "pid", pid, "error", err)
		return
	}

	slog.Info("killing stale magicskills instance", "pid", pid)

	// os.Process.Kill() sends SIGKILL on Unix, TerminateProcess on Windows.
	// We use Kill() directly because the stale process is unresponsive to
	// graceful signals (its stdin is already severed).
	if err := proc.Kill(); err != nil {
		slog.Debug("failed to kill stale process (may have already exited)", "pid", pid, "error", err)
	}

	// Reap the process to avoid zombies (no-op on Windows).
	_, _ = proc.Wait()
}

// ShutdownDeadline starts a background goroutine that will force-exit the
// process if cleanup takes longer than the specified duration. This is the
// final safety net to prevent the process from hanging on shutdown.
func ShutdownDeadline(d time.Duration) {
	go func() {
		time.Sleep(d)
		slog.Warn("shutdown deadline exceeded, forcing exit", "deadline", d)
		os.Exit(0)
	}()
}
