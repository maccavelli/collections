// Package lifecycle provides functionality for the lifecycle subsystem.
package lifecycle

import (
	"context"
	"log/slog"
	"os"
	"time"
)

const (
	// parentPollInterval is how often the watchdog checks if the parent
	// process is still alive.
	parentPollInterval = 2 * time.Second
)

// WatchParent monitors the parent process and cancels the provided context
// when the parent dies. This detects orphaned MCP server processes that
// survive IDE reloads.
//
// On Linux/macOS, when the parent dies the child is reparented to init (PID 1).
// On Windows, the parent PID stays the same but the process no longer exists.
//
// The watchdog is a no-op if parentPID is 0 or 1 (launched directly by init).
func WatchParent(ctx context.Context, cancel context.CancelFunc) {
	parentPID := os.Getppid()

	// If we're already a top-level process, there's nothing to watch.
	if parentPID <= 1 {
		slog.Debug("parent watchdog disabled (top-level process)", "ppid", parentPID)
		return
	}

	slog.Debug("parent watchdog started", "parent_pid", parentPID)

	ticker := time.NewTicker(parentPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !isParentAlive(parentPID) {
				slog.Warn("parent process died, initiating graceful shutdown", "parent_pid", parentPID)
				cancel()
				return
			}
		}
	}
}
