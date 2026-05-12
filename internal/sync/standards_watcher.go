// Package sync provides functionality for the sync subsystem.
package sync

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"mcp-server-magicdev/internal/config"
	"mcp-server-magicdev/internal/db"
)

// StartStandardsWatcher creates fsnotify watchers on the configured standards
// directories and automatically updates BuntDB when local .md files change.
// It uses per-file debounce timers (500ms) to prevent reading partially-written
// files, and SHA-256 hash comparison to avoid unnecessary BuntDB writes.
//
// The watcher integrates with the config hot-reload system to automatically
// pick up new directory paths when magicdev.yaml changes.
func StartStandardsWatcher(ctx context.Context, wg *gosync.WaitGroup, store *db.Store) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("failed to create standards watcher, live reload disabled", "error", err)
		return
	}

	// Read initial directory paths from config and start watching.
	paths := collectStandardsPaths()
	watchedCount := 0
	for _, dir := range paths {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			if err := watcher.Add(dir); err != nil {
				slog.Warn("failed to watch standards directory", "dir", dir, "error", err)
			} else {
				watchedCount++
				slog.Info("watching standards directory for changes", "dir", dir)
			}
		} else {
			slog.Debug("standards directory does not exist, skipping watch", "dir", dir)
		}
	}

	if watchedCount == 0 {
		slog.Info("no standards directories to watch")
		watcher.Close()
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer watcher.Close()

		// Per-file debounce timers to prevent reading partially-written files
		// and to coalesce rapid sequential writes to the same file.
		timers := make(map[string]*time.Timer)

		for {
			select {
			case <-ctx.Done():
				// Cancel all pending timers on shutdown.
				for _, t := range timers {
					t.Stop()
				}
				slog.Debug("standards watcher shutting down")
				return

			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if !isRelevantStandardsEvent(event) {
					continue
				}

				filePath := event.Name

				// Cancel previous timer for this file if still pending.
				if t, exists := timers[filePath]; exists {
					t.Stop()
				}

				// Start a new 500ms debounce timer for this file.
				timers[filePath] = time.AfterFunc(500*time.Millisecond, func() {
					refreshLocalStandard(store, filePath)
				})

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Error("standards watcher error", "error", err)
			}
		}
	}()

	// Register a config hot-reload hook to pick up new directory paths
	// when the user modifies magicdev.yaml.
	config.OnConfigReload = append(config.OnConfigReload, func() {
		newPaths := collectStandardsPaths()
		for _, dir := range newPaths {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				// watcher.Add is idempotent — adding an already-watched dir is a no-op.
				if err := watcher.Add(dir); err != nil {
					slog.Warn("failed to watch new standards directory", "dir", dir, "error", err)
				} else {
					slog.Debug("standards watcher updated with new directory", "dir", dir)
				}
			}
		}
	})

	slog.Info("standards live-reload watcher started", "directories", watchedCount)
}

// collectStandardsPaths reads the standards directory paths from the current
// viper configuration for all stacks.
func collectStandardsPaths() []string {
	var paths []string
	for _, stack := range []string{"node", "dotnet"} {
		key := "standards." + stack + ".path"
		if p := viper.GetString(key); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// isRelevantStandardsEvent returns true if the fsnotify event should trigger
// a cache refresh. Only Write, Create, and Rename events on .md files are relevant.
func isRelevantStandardsEvent(event fsnotify.Event) bool {
	// Only process markdown files.
	if !strings.HasSuffix(event.Name, ".md") {
		return false
	}

	// Ignore temp files, swap files, and editor backups.
	base := filepath.Base(event.Name)
	if strings.HasPrefix(base, ".") || strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".swp") {
		return false
	}

	// Accept Write, Create, and Rename (atomic save) events.
	return event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0
}

// refreshLocalStandard re-reads a local .md file, computes its SHA-256,
// compares to the BuntDB-cached hash, and updates the cache if the content changed.
func refreshLocalStandard(store *db.Store, path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		// File may have been deleted — this is not an error worth escalating.
		slog.Debug("standards watcher: could not read file (may have been deleted)",
			"path", path,
			"error", err,
		)
		return
	}

	if len(content) == 0 {
		slog.Debug("standards watcher: ignoring empty file", "path", path)
		return
	}

	diskHash := sha256Hex(content)
	cachedHash, _ := store.GetBaselineHash(path)

	if diskHash == cachedHash {
		slog.Debug("standards watcher: file unchanged (hash match)", "path", path)
		return
	}

	if err := storeBaseline(store, path, content); err != nil {
		slog.Error("standards watcher: failed to update BuntDB cache",
			"path", path,
			"error", err,
		)
		return
	}

	slog.Info("standards watcher: live-reloaded standard into BuntDB",
		"path", path,
		"old_hash", truncateHash(cachedHash),
		"new_hash", diskHash[:8],
		"bytes", len(content),
	)
}

// truncateHash safely truncates a hash string for log display.
func truncateHash(h string) string {
	if len(h) >= 8 {
		return h[:8]
	}
	if h == "" {
		return "(new)"
	}
	return h
}
