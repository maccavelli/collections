package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"mcp-server-recall/internal/config"
	"mcp-server-recall/internal/memory"
)

var (
	ringMu    sync.Mutex
	ringBytes []byte
)

func StartTelemetryLoop(cfg *config.Config, store *memory.MemoryStore, logStream func() string) {
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		for range ticker.C {
			WriteSnapshot(cfg, store, logStream)
		}
	}()
}

func WriteSnapshot(cfg *config.Config, store *memory.MemoryStore, logStream func() string) {
	ringMu.Lock()
	defer ringMu.Unlock()

	// Gather metrics
	cacheHit, cacheMiss, dbHit, dbMiss := store.GetTelemetry()
	mmCount, sCount, stCount, pCount := store.GetNamespaceCounts()
	docs, _ := store.DocCount()

	stats := make(map[string]any)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	snapshot := map[string]any{
		"storage": stats,
		"bleve": map[string]any{
			"documents": docs,
			"queues":    0,
			"drift":     store.DriftAlerts(),
		},
		"taxonomy": map[string]any{
			"memories":  mmCount,
			"sessions":  sCount,
			"standards": stCount,
			"projects":  pCount,
		},
		"analytics": map[string]any{
			"cache_hits":   cacheHit,
			"cache_misses": cacheMiss,
			"db_hits":      dbHit,
			"db_misses":    dbMiss,
		},
		"ast": map[string]any{
			"disable_drift": cfg.HarvestDisableDrift(),
			"exclude_dirs": len(cfg.ExcludeDirs()),
		},
		"config": map[string]any{
			"db_path": cfg.GetDBPath(),
			"version": cfg.Version,
		},
		"runtime": map[string]any{
			"memory_mb":  m.Alloc / 1024 / 1024,
			"goroutines": runtime.NumGoroutine(),
		},
	}

	snapBytes, _ := json.Marshal(snapshot)
	logData := logStream()

	// Write atomically to telemetry.ring
	path := filepath.Join(cfg.GetDBPath(), "telemetry.ring")
	tmpPath := path + ".tmp"
	
	// Format: Single Line JSON \n Log Lines
	payload := []byte(fmt.Sprintf("%s\n%s", string(snapBytes), logData))
	
	_ = os.WriteFile(tmpPath, payload, 0644)
	_ = os.Rename(tmpPath, path)
}
