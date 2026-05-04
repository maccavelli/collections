package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

const (
	Name          = "mcp-server-recall"
	DefaultDBName = ".mcp_recall"
	EnvPrefix     = "MCP_RECALL"

	// Logging: get_internal_logs buffer limits
	LogBufferLimit  = 1024 * 1024 // 1MB max log buffer
	LogTrimTarget   = 512 * 1024  // 512KB trim target
	DefaultLogLines = 25          // Default lines returned by get_internal_logs
)

// BatchConfig holds tunable options for SaveBatch and harvest/ingest operations.
type BatchConfig struct {
	MaxBatchSize             int `mapstructure:"max_batch_size"`
	HarvestChunkSize         int `mapstructure:"harvest_chunk_size"`
	HarvestInterBatchSleepMs int `mapstructure:"harvest_inter_batch_sleep_ms"`
	IngestInterBatchSleepMs  int `mapstructure:"ingest_inter_batch_sleep_ms"`
	LoadFastWritesEnabled    int `mapstructure:"load_fast_writes_enabled"`
}

// HarvestConfig holds tunable settings for directory ingestion rules.
type HarvestConfig struct {
	DisableDrift bool     `mapstructure:"disable_drift"`
	ExcludeDirs  []string `mapstructure:"exclude_dirs"`
}

// State holds the actual configuration values mapped to yaml.
type State struct {
	Name              string        `mapstructure:"name"`
	Version           string        `mapstructure:"version"`
	DBPath            string        `mapstructure:"dbPath"`
	ExportDir         string        `mapstructure:"exportDir"`
	SearchEnabled     bool          `mapstructure:"searchEnabled"`
	SearchLimit       int           `mapstructure:"searchLimit"`
	EncryptionKey     string        `mapstructure:"encryptionKey"`
	DedupThreshold    float64       `mapstructure:"dedupThreshold"`
	APIPort           int           `mapstructure:"apiPort"`
	Harvest           HarvestConfig `mapstructure:"harvest"`
	SafeTools         []string      `mapstructure:"safeTools"`
	SafeToolsInternal []string      `mapstructure:"safeToolsInternal"`
	Batch             BatchConfig   `mapstructure:"batchsettings"`
	SessionPurgeDays  int           `mapstructure:"sessionpurgedays"`
}

// Config safely wraps Viper state with an RWMutex for hot-reloads.
type Config struct {
	mu      sync.RWMutex
	state   State
	Version string
}

// New initializes the Viper bindings, sets up OS-agnostic paths, and attaches the fsnotify hook.
func New(version string) *Config {
	cfg := &Config{
		Version: version,
	}

	viper.SetEnvPrefix(EnvPrefix)
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Explicit bindings to accommodate both camelCase YAML parsing and standard underscore bash environments.
	if err := viper.BindEnv("encryptionKey", "MCP_RECALL_ENCRYPTION_KEY", "MCP_RECALL_ENCRYPTIONKEY"); err != nil {
		slog.Warn("failed to bind encryption key environment variable", "error", err)
	}

	// Cross-compile safe OS mappings completely decoupled from unix tildes
	configDir, err := os.UserConfigDir()
	if err != nil {
		slog.Warn("failed to isolate OS UserConfigDir; falling back to current working directory", "error", err)
		configDir = "."
	}
	appConfigDir := filepath.Join(configDir, Name)

	viper.SetConfigName("recall")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(appConfigDir)
	viper.AddConfigPath(".")

	// Set Defaults
	viper.SetDefault("name", Name)
	viper.SetDefault("version", version)
	viper.SetDefault("dbPath", filepath.Join(appConfigDir, DefaultDBName))
	viper.SetDefault("exportDir", os.TempDir())
	viper.SetDefault("searchEnabled", true)
	viper.SetDefault("searchLimit", 25000)
	viper.SetDefault("dedupThreshold", 0.8)
	viper.SetDefault("apiPort", 0)
	viper.SetDefault("sessionpurgedays", 5)

	// Batch settings defaults
	viper.SetDefault("batchsettings.max_batch_size", 100)
	viper.SetDefault("batchsettings.harvest_chunk_size", 50)
	viper.SetDefault("batchsettings.harvest_inter_batch_sleep_ms", 500)
	viper.SetDefault("batchsettings.ingest_inter_batch_sleep_ms", 50)
	viper.SetDefault("batchsettings.load_fast_writes_enabled", 0)

	// Native substitution for hardcoded engine noise arrays
	viper.SetDefault("harvest.exclude_dirs", []string{"/vendor/", "/testdata/", "/mocks", "/internal/logs", "/tests", "/cmd/"})
	viper.SetDefault("harvest.disable_drift", false)

	// Default SafeTools dynamically exposed to read-only Streamable HTTP endpoint
	viper.SetDefault("safeTools", []string{
		"save_sessions",
		"search",
		"get",
		"list",
	})

	// Default SafeToolsInternal explicitly bypassing restrictions for the internal CLI
	viper.SetDefault("safeToolsInternal", []string{
		"recall",
		"batch_recall",
		"export_records",
		"import_records",
		"save_sessions",
		"search",
		"get",
		"list",
		"harvest",
		"delete",
		"prune_records",
		"forget",
		"reload_cache",
		"get_internal_logs",
		"get_metrics",
		"recall_recent",
	})

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			slog.Debug("no recall.yaml config file found; relying on defaults and environment variables")
		} else {
			slog.Warn("error parsing recall.yaml", "error", err)
		}
	}

	cfg.refreshState()

	// Enable True Hot-Reloading Sequence
	viper.WatchConfig()
	var lastConfigUpdate time.Time
	var debounceMu sync.Mutex

	viper.OnConfigChange(func(e fsnotify.Event) {
		debounceMu.Lock()
		if time.Since(lastConfigUpdate) < 500*time.Millisecond {
			debounceMu.Unlock()
			return
		}
		lastConfigUpdate = time.Now()
		debounceMu.Unlock()

		slog.Info("[Viper] Configuration file modified dynamically", "file", e.Name)
		cfg.refreshState()
		slog.Info("[Viper] Configuration reloaded into memory", "disable_drift_applied_status", cfg.HarvestDisableDrift())
	})

	return cfg
}

func (c *Config) refreshState() {
	var newState State
	if err := viper.Unmarshal(&newState); err != nil {
		slog.Error("failed to unmarshal viper configuration", "error", err)
		return
	}

	// Always enforce the binary-linked version
	newState.Name = Name
	newState.Version = c.Version

	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = newState
}

// Thread-safe accessors for cross-application mapping

func (c *Config) GetDBPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p := c.state.DBPath
	if filepath.IsAbs(p) {
		return p
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

func (c *Config) ExportDir() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.ExportDir
}

func (c *Config) SearchEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.SearchEnabled
}

func (c *Config) SearchLimit() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.SearchLimit
}

func (c *Config) EncryptionKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.EncryptionKey
}

func (c *Config) APIPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.APIPort
}

func (c *Config) ExcludeDirs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// Return a copy to prevent mutation bugs out-of-scope
	dirs := make([]string, len(c.state.Harvest.ExcludeDirs))
	copy(dirs, c.state.Harvest.ExcludeDirs)
	return dirs
}

func (c *Config) Name() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.Name
}

func (c *Config) SafeTools() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	tools := make([]string, len(c.state.SafeTools))
	copy(tools, c.state.SafeTools)
	return tools
}

func (c *Config) SafeToolsInternal() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	tools := make([]string, len(c.state.SafeToolsInternal))
	copy(tools, c.state.SafeToolsInternal)
	return tools
}

func (c *Config) DedupThreshold() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.DedupThreshold
}

func (c *Config) BatchSettings() BatchConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.Batch
}
func (c *Config) HarvestDisableDrift() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.Harvest.DisableDrift
}

func (c *Config) SessionPurgeDays() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.SessionPurgeDays
}
