package persistent

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/tidwall/buntdb"
)

// OpenDB initializes and configures the persistent BuntDB cache.
func OpenDB(name string) (*buntdb.DB, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("get user cache directory: %w", err)
	}

	appCacheDir := filepath.Join(cacheDir, name)
	if err := os.MkdirAll(appCacheDir, 0o750); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	dbPath := filepath.Join(appCacheDir, "cache.db")
	db, err := buntdb.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open buntdb: %w", err)
	}

	var bconfig buntdb.Config
	if err := db.ReadConfig(&bconfig); err == nil {
		bconfig.SyncPolicy = buntdb.EverySecond
		bconfig.AutoShrinkPercentage = 50
		bconfig.AutoShrinkMinSize = 25 * 1024 * 1024
		if setErr := db.SetConfig(bconfig); setErr != nil {
			slog.Warn("failed to apply buntdb cache configuration", "error", setErr)
		}
	} else {
		slog.Warn("failed to read buntdb cache configuration", "error", err)
	}

	return db, nil
}
