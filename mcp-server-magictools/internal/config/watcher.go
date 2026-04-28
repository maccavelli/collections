package config

import (
	"crypto/sha256"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// ConfigChangeHandler is called when the managed server set changes
type ConfigChangeHandler interface {
	// OnServerPromoted is called when a server transitions from disabled->enabled
	// (magictools loses ownership). Its tools should be purged from the index.
	OnServerPromoted(name string)

	// OnServerDemoted is called when a server transitions from enabled->disabled
	// (magictools gains ownership). Available for next sync_ecosystem.
	OnServerDemoted(name string)

	// OnConfigReloaded is called when the configuration file has been re-read.
	OnConfigReloaded(cfg *Config)

	// OnMCPLogLevelChanged is called when the global MCPLogLevel configuration changes.
	OnMCPLogLevelChanged(oldLevel, newLevel string)
}

// Watcher monitors config.yaml (via Viper) and servers.yaml (via fsnotify)
type Watcher struct {
	v          *viper.Viper
	liveConfig *Config
	handler    ConfigChangeHandler
	current    map[string]bool // current managed server names
	mu         sync.Mutex
	stop       chan struct{}
	lastHash   [32]byte // config.yaml hash
	srvHash    [32]byte // servers.yaml hash
}

// NewWatcher creates a config file watcher
func NewWatcher(v *viper.Viper, cfg *Config, handler ConfigChangeHandler) *Watcher {
	return &Watcher{
		v:          v,
		liveConfig: cfg,
		handler:    handler,
		current:    cfg.GetManagedServerNames(),
		stop:       make(chan struct{}),
	}
}

// Start begins watching config.yaml and servers.yaml for changes.
func (w *Watcher) Start() {
	// Watch config.yaml via Viper (orchestrator settings)
	w.v.OnConfigChange(func(e fsnotify.Event) {
		slog.Info("file changed", "component", "config", "path", e.Name, "op", e.Op.String())
		w.handleChange()
	})
	w.v.WatchConfig()
	slog.Info("watcher started", "component", "config", "path", w.v.ConfigFileUsed())

	// Watch servers.yaml via fsnotify (sub-server registry)
	serversPath := filepath.Join(DefaultConfigDir(), ServersConfigFile)
	go w.watchServersFile(serversPath)

	// Hardening: Fallback polling for Bastion hosts where fsnotify might fail
	go func(stop chan struct{}) {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				w.handleChange()
				w.handleServersChange()
			}
		}
	}(w.stop)
}

// watchServersFile uses fsnotify to watch servers.yaml for changes.
func (w *Watcher) watchServersFile(path string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("servers.yaml: fsnotify unavailable, relying on polling", "error", err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(filepath.Dir(path)); err != nil {
		slog.Warn("servers.yaml: failed to watch directory, relying on polling", "error", err)
		return
	}

	slog.Info("servers.yaml watcher started", "component", "config", "path", path)
	for {
		select {
		case <-w.stop:
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Base(event.Name) == ServersConfigFile && (event.Op&(fsnotify.Write|fsnotify.Create)) != 0 {
				slog.Info("servers.yaml changed", "component", "config", "op", event.Op.String())
				w.handleServersChange()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("servers.yaml: fsnotify error", "error", err)
		}
	}
}

// Stop signals the watcher to shut down
func (w *Watcher) Stop() {
	select {
	case <-w.stop:
		// already closed
	default:
		close(w.stop)
	}
}

// UpdateManaged replaces the current managed set (called after sync_ecosystem)
func (w *Watcher) UpdateManaged(managed map[string]bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current = managed
}

// handleChange processes config.yaml changes (orchestrator settings only).
func (w *Watcher) handleChange() {
	// 🛡️ HASH CHECK: Skip redundant reloads when the config file hasn't changed.
	raw, err := os.ReadFile(w.v.ConfigFileUsed())
	if err != nil {
		slog.Error("failed to read config file", "component", "config", "error", err)
		return
	}
	newHash := sha256.Sum256(raw)
	w.mu.Lock()
	if newHash == w.lastHash {
		w.mu.Unlock()
		return
	}
	w.lastHash = newHash
	w.mu.Unlock()

	cfg, err := LoadFromViper(w.v)
	if err != nil {
		slog.Error("failed to reload config", "component", "config", "error", err)
		return
	}

	// Update orchestrator settings (NOT managed servers — those come from servers.yaml)
	w.liveConfig.mu.Lock()
	oldMCPLogLevel := w.liveConfig.MCPLogLevel
	w.liveConfig.MaxResponseTokens = cfg.MaxResponseTokens
	w.liveConfig.LogLevel = cfg.LogLevel
	w.liveConfig.MCPLogLevel = cfg.MCPLogLevel
	w.liveConfig.SqueezeLevelState = cfg.SqueezeLevelState
	w.liveConfig.ScoreThreshold = cfg.ScoreThreshold
	w.liveConfig.ValidateProxyCalls = cfg.ValidateProxyCalls
	w.liveConfig.SqueezeBypass = cfg.SqueezeBypass
	w.liveConfig.PinnedServers = cfg.PinnedServers
	w.liveConfig.Intelligence = cfg.Intelligence
	w.liveConfig.mu.Unlock()

	w.handler.OnConfigReloaded(w.liveConfig)
	// 🛡️ TRANSIENT STATE VALIDATION: sed -i creates a new file and moves it, triggering multiple
	// fsnotify events. Viper can occasionally read the file during the microsecond it is empty or half-written,
	// causing MCPLogLevel to unmarshal as "". If we accept this, it triggers a massive false-positive ecosystem
	// restart. We must explicitly require that BOTH the old and new levels are fully populated to prevent this.
	if cfg.MCPLogLevel != "" && oldMCPLogLevel != "" && oldMCPLogLevel != cfg.MCPLogLevel {
		slog.Warn("sub-server log level mutated; notifying fleet controller", "component", "config", "old", oldMCPLogLevel, "new", cfg.MCPLogLevel)
		w.handler.OnMCPLogLevelChanged(oldMCPLogLevel, cfg.MCPLogLevel)
	} else if cfg.MCPLogLevel == "" && oldMCPLogLevel != "" {
		// Transient empty read detected. Do not trigger a reload. The next fully-written config event
		// will overwrite the in-memory value back to its stable state.
		slog.Debug("transient empty config read filtered to prevent false-positive reload cascade", "component", "config")
	}
}

// handleServersChange processes servers.yaml changes (sub-server registry).
func (w *Watcher) handleServersChange() {
	path := filepath.Join(DefaultConfigDir(), ServersConfigFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		slog.Error("failed to read servers.yaml", "component", "config", "error", err)
		return
	}
	newHash := sha256.Sum256(raw)
	w.mu.Lock()
	if newHash == w.srvHash {
		w.mu.Unlock()
		return
	}
	w.srvHash = newHash
	w.mu.Unlock()

	servers, err := LoadManagedServers()
	if err != nil {
		slog.Error("failed to reload servers.yaml", "component", "config", "error", err)
		return
	}

	// Update the live config with new server list
	w.liveConfig.UpdateManagedServers(servers)
	newManaged := w.liveConfig.GetManagedServerNames()

	w.mu.Lock()
	oldManaged := w.current
	w.current = newManaged
	w.mu.Unlock()

	// Detect servers removed from servers.yaml
	for name := range oldManaged {
		if !newManaged[name] {
			slog.Info("server removed from servers.yaml", "component", "config", "server_id", name)
			w.handler.OnServerPromoted(name)
		}
	}

	// Detect servers added to servers.yaml
	for name := range newManaged {
		if !oldManaged[name] {
			slog.Info("server added to servers.yaml", "component", "config", "server_id", name)
			w.handler.OnServerDemoted(name)
		}
	}

	slog.Info("servers.yaml reloaded", "component", "config", "servers", len(servers))
}
