package config

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const (
	Name              = "mcp-server-magictools"
	AppName           = "mcp-server-magictools"
	LegacyDBPath      = ".mcp_magictools" // Pre-migration dot-directory in $HOME
	EnvDBPath         = "MCP_MAGIC_TOOLS_DB_PATH"
	EnvConfigPath     = "MCP_MAGIC_TOOLS_CONFIG"
	EnvConfigDir      = "MCP_MAGIC_TOOLS_CONFIG_DIR"
	SelfName          = "magictools"
	ToolConfigFile    = "config.yaml"
	ServersConfigFile = "servers.yaml"
)

// ResolveAPIURLs canonicalizes the comma-separated MCP_API_URL environment variable.
// This follows the same convention used by brainstorm and go-refactor sub-servers
// to locate the recall HTTP SSE endpoint.
func ResolveAPIURLs() []string {
	val := os.Getenv("MCP_API_URL")
	if val == "" {
		return nil
	}

	raw := strings.Split(val, ",")
	var cleaned []string
	for _, u := range raw {
		u = strings.TrimSpace(u)
		if u != "" {
			cleaned = append(cleaned, u)
		}
	}
	return cleaned
}

// DefaultConfigDir returns the platform-native configuration directory.
//   - Linux: ~/.config/mcp-server-magictools
//   - macOS: ~/Library/Application Support/mcp-server-magictools
func DefaultConfigDir() string {
	if dir := os.Getenv(EnvConfigDir); dir != "" {
		return dir
	}
	base, err := os.UserConfigDir()
	if err != nil {
		// Fallback: behaves identically to the legacy path on Linux
		if home, herr := os.UserHomeDir(); herr == nil {
			return filepath.Join(home, ".config", AppName)
		}
		return filepath.Join("/tmp", AppName)
	}
	return filepath.Join(base, AppName)
}

// DefaultCacheDir returns the platform-native cache directory.
//   - Linux: ~/.cache/mcp-server-magictools
//   - macOS: ~/Library/Caches/mcp-server-magictools
func DefaultCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		if home, herr := os.UserHomeDir(); herr == nil {
			return filepath.Join(home, ".cache", AppName)
		}
		return filepath.Join("/tmp", AppName)
	}
	return filepath.Join(base, AppName)
}

// DefaultLogPath returns the platform-native log file path.
//   - Linux: ~/.cache/mcp-server-magictools/magictools_debug.log
//   - macOS: ~/Library/Caches/mcp-server-magictools/magictools_debug.log
func DefaultLogPath() string {
	return filepath.Join(DefaultCacheDir(), "magictools_debug.log")
}

// DefaultDataPath returns the platform-native data directory for BadgerDB.
//   - Linux: ~/.config/mcp-server-magictools/db
//   - macOS: ~/Library/Application Support/mcp-server-magictools/db
func DefaultDataPath() string {
	return filepath.Join(DefaultConfigDir(), "db")
}

// MigrateDataDir performs a one-time atomic migration of data from oldPath to newPath.
// It is a no-op if oldPath doesn't exist or newPath already exists.
func MigrateDataDir(oldPath, newPath string) error {
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return nil // nothing to migrate
	}
	if _, err := os.Stat(newPath); err == nil {
		return nil // new path already exists, skip
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return fmt.Errorf("migration: failed to create parent dir: %w", err)
	}
	slog.Info("migrating data directory", "from", oldPath, "to", newPath)
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("migration: failed to move data: %w", err)
	}
	slog.Info("migration complete", "new_path", newPath)
	return nil
}

// IntelligenceEngine holds configuration for both the generative hydrator and the embedding engine.
type IntelligenceEngine struct {
	// Generative Hydrator settings
	Provider       string   `json:"provider,omitzero" mapstructure:"provider" yaml:"provider,omitempty"`
	Model          string   `json:"model,omitzero" mapstructure:"model" yaml:"model,omitempty"`
	APIKey         string   `json:"api_key,omitzero" mapstructure:"api_key" yaml:"api_key,omitempty"`
	FallbackModels []string `json:"fallback_models,omitzero" mapstructure:"fallback_models" yaml:"fallback_models,omitempty"`
	RetryCount     int      `json:"retry_count,omitzero" mapstructure:"retry_count" yaml:"retry_count,omitempty"`
	RetryDelay     int      `json:"retry_delay_seconds,omitzero" mapstructure:"retry_delay_seconds" yaml:"retry_delay_seconds,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitzero" mapstructure:"timeout_seconds" yaml:"timeout_seconds,omitempty"`

	// Embedding Engine settings (decoupled from generative hydrator)
	EmbeddingProvider      string `json:"embedding_provider,omitzero" mapstructure:"embedding_provider" yaml:"embedding_provider,omitempty"`
	EmbeddingModel         string `json:"embedding_model,omitzero" mapstructure:"embedding_model" yaml:"embedding_model,omitempty"`
	EmbeddingAPIKey        string `json:"embedding_api_key,omitzero" mapstructure:"embedding_api_key" yaml:"embedding_api_key,omitempty"`
	EmbeddingAPIURL        string `json:"embedding_api_url,omitzero" mapstructure:"embedding_api_url" yaml:"embedding_api_url,omitempty"`
	EmbeddedDimensionality int    `json:"embedded_dimensionality,omitzero" mapstructure:"embedded_dimensionality" yaml:"embedded_dimensionality,omitempty"`
}

// ConfigurationBlock is undocumented but satisfies standard structural requirements.
type ConfigurationBlock struct {
	MaxResponseTokens    int                `json:"maxResponseTokens,omitzero" mapstructure:"maxResponseTokens" yaml:"maxResponseTokens,omitempty"`
	SqueezeLevel         *int               `json:"squeezeLevel,omitzero" mapstructure:"squeezeLevel" yaml:"squeezeLevel,omitempty"`
	LogFormat            string             `json:"logFormat,omitzero" mapstructure:"logFormat" yaml:"logFormat,omitempty"`
	LogLevel             string             `json:"logLevel,omitzero" mapstructure:"logLevel" yaml:"logLevel,omitempty"`
	MCPLogLevel          string             `json:"mcpLogLevel,omitzero" mapstructure:"mcpLogLevel" yaml:"mcpLogLevel,omitempty"`
	ScoreThreshold       float64            `json:"scoreThreshold,omitzero" mapstructure:"scoreThreshold" yaml:"scoreThreshold,omitempty"`
	ValidateProxyCalls   *bool              `json:"validateProxyCalls,omitzero" mapstructure:"validateProxyCalls" yaml:"validateProxyCalls,omitempty"`
	SqueezeBypass        []string           `json:"squeezeBypass,omitzero" mapstructure:"squeezeBypass" yaml:"squeezeBypass,omitempty"`
	RingBufferTargets    []string           `json:"ringBufferTargets,omitzero" mapstructure:"ringBufferTargets" yaml:"ringBufferTargets,omitempty"`
	PinnedServers        []string           `json:"pinnedServers,omitzero" mapstructure:"pinnedServers" yaml:"pinnedServers,omitempty"`
	TokenSpendThresh     int                `json:"tokenSpendThresh,omitzero" mapstructure:"tokenSpendThresh" yaml:"tokenSpendThresh,omitempty"`
	LRULimit             int                `json:"lruLimit,omitzero" mapstructure:"lruLimit" yaml:"lruLimit,omitempty"`
	SynthesisBiasVector  float64            `json:"synthesisBiasVector,omitzero" mapstructure:"synthesisBiasVector" yaml:"synthesisBiasVector,omitempty"`
	SynthesisBiasSynergy float64            `json:"synthesisBiasSynergy,omitzero" mapstructure:"synthesisBiasSynergy" yaml:"synthesisBiasSynergy,omitempty"`
	SynthesisBiasRole    float64            `json:"synthesisBiasRole,omitzero" mapstructure:"synthesisBiasRole" yaml:"synthesisBiasRole,omitempty"`
	ScoreFusionAlpha     float64            `json:"scoreFusionAlpha,omitzero" mapstructure:"scoreFusionAlpha" yaml:"scoreFusionAlpha,omitempty"`
	Intelligence         IntelligenceEngine `json:"intelligence,omitzero" mapstructure:"intelligence" yaml:"intelligence,omitempty"`
}

// IDEConfig matches the IDE's mcp_config.json top-level structure
type IDEConfig struct {
	McpServers    map[string]IDEServerEntry `json:"mcpServers" mapstructure:"mcpServers"`
	Configuration ConfigurationBlock        `json:"configuration,omitzero" mapstructure:"configuration"`
}

// IDEServerEntry matches a single server entry in the IDE config
type IDEServerEntry struct {
	Command       string            `json:"command" mapstructure:"command"`
	Args          []string          `json:"args" mapstructure:"args"`
	Env           map[string]string `json:"env,omitzero" mapstructure:"env"`
	Disabled      bool              `json:"disabled" mapstructure:"disabled"`
	DisabledTools []string          `json:"disabledTools,omitzero" mapstructure:"disabledTools"`
	LogLevel      string            `json:"logLevel,omitzero" mapstructure:"logLevel"`
}

// ServerConfig defines a magictools-managed sub-server (derived from IDE entries with disabled: true)
type ServerConfig struct {
	Name          string
	Command       string
	Args          []string
	Env           map[string]string
	DisabledTools []string
	MemoryLimitMB int
	GoMemLimitMB  int
	MaxCPULimit   int
	DeferredBoot  bool
}

// Configuration options shared with SaveInternalTools
type PersistentConfig struct {
	SqueezeLevel *int
}

// Hash returns a stable hash of the server configuration
func (s ServerConfig) Hash() string {
	data, err := json.Marshal(s)
	if err != nil {
		return "invalid-config-hash"
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// Config holds the application configuration
type Config struct {
	mu sync.RWMutex

	Name                 string
	Version              string
	DBPath               string
	ConfigPath           string
	ManagedServers       []ServerConfig
	ideConfig            *IDEConfig // raw parsed config for diffing
	MaxResponseTokens    int
	LogPath              string
	LogFormat            string // format style: json or text
	NoOptimize           bool   // Flag to disable SqueezeWriter and description minification
	Debug                bool   // Flag for high-frequency trace logging and foreground mode
	LogLevel             string // Configured log level (ERROR, WARN, INFO, DEBUG, TRACE)
	MCPLogLevel          string // Configured sub-server log level
	SqueezeLevelState    *int   // Retained so SaveInternalTools doesn't wipe it
	ScoreThreshold       float64
	ValidateProxyCalls   bool               // Toggle JSON schema validation in call_proxy (default: true)
	SqueezeBypass        []string           // Array of server names allowed to bypass the minifier
	RingBufferTargets    []string           // Array of specific targets (server:tool or server) routed to CSSA Ring Buffer natively
	PinnedServers        []string           // Servers exempt from idle eviction and LRU eviction
	TokenSpendThresh     int                // Session LLM token circuit-breaker limit
	LRULimit             int                // Max items for ResponseCache and RegistryCache
	SynthesisBiasVector  float64            // Engine weight in tri-factor scoring
	SynthesisBiasSynergy float64            // Synergy (ghost index) weight in tri-factor scoring
	SynthesisBiasRole    float64            // Role boost weight in tri-factor scoring
	ScoreFusionAlpha     float64            // Vector weight in direct score fusion (0.0=pure BM25, 1.0=pure vector)
	Intelligence         IntelligenceEngine // Native HTTP configuration for the Hydrator daemon
	v                    *viper.Viper
}

// GetManagedServers returns a copy of the managed servers slice thread-safely
func (c *Config) GetManagedServers() []ServerConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	res := make([]ServerConfig, len(c.ManagedServers))
	copy(res, c.ManagedServers)
	return res
}

// GetLogFormat returns the current orchestrator log format thread-safely
func (c *Config) GetLogFormat() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.LogFormat == "" {
		return "json" // Default to JSON strict constraint
	}
	return c.LogFormat
}

// GetLogLevel returns the current orchestrator log level thread-safely
func (c *Config) GetLogLevel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.LogLevel
}

// GetMCPLogLevel returns the current sub-server log level thread-safely
func (c *Config) GetMCPLogLevel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.MCPLogLevel == "" {
		return c.LogLevel
	}
	return c.MCPLogLevel
}

// GetSqueezeBypass returns a copy of the minifier bypass slice thread-safely
func (c *Config) GetSqueezeBypass() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	res := make([]string, len(c.SqueezeBypass))
	copy(res, c.SqueezeBypass)
	return res
}

// GetRingBufferTargets returns a copy of the ring buffer targets slice thread-safely
func (c *Config) GetRingBufferTargets() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	res := make([]string, len(c.RingBufferTargets))
	copy(res, c.RingBufferTargets)
	return res
}

// GetPinnedServers returns a copy of the pinned servers slice thread-safely
func (c *Config) GetPinnedServers() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	res := make([]string, len(c.PinnedServers))
	copy(res, c.PinnedServers)
	return res
}

// UpdateManagedServers updates the managed servers slice thread-safely
func (c *Config) UpdateManagedServers(servers []ServerConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ManagedServers = servers
}

// New initializes configuration with path discovery: flag -> env -> default
func New(version, flagPath string) (*Config, error) {
	dbPath := os.Getenv(EnvDBPath)
	if dbPath == "" {
		dbPath = DefaultDataPath()

		// 🛡️ AUTO-MIGRATION: Move legacy ~/.mcp_magictools to the new platform-native location.
		if home, err := os.UserHomeDir(); err == nil {
			oldPath := filepath.Join(home, LegacyDBPath)
			if err := MigrateDataDir(oldPath, dbPath); err != nil {
				slog.Warn("config: legacy data migration failed", "error", err)
			}
		}
	}

	configPath := flagPath
	if configPath == "" {
		configPath = os.Getenv(EnvConfigPath)
	}
	if configPath == "" {
		configPath = filepath.Join(DefaultConfigDir(), ToolConfigFile)
	}

	// 🛡️ ABSOLUTE RESOLUTION: Ensure we are using an absolute path before loading
	if abs, err := filepath.Abs(configPath); err == nil {
		configPath = abs
	}

	// 🛡️ VIPER INITIALIZATION: Set up Viper to manage the primary config file
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetEnvPrefix("MAGIC_TOOLS")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// If it exists but failed to read, error out
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to read config: %w", err)
			}
		}
		// Auto-create default configuration if missing
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			slog.Warn("config: failed to create config directory", "path", configPath, "error", err)
		}

		v.SetDefault("configuration.squeezeLevel", 3)
		v.SetDefault("configuration.logLevel", "DEBUG")
		v.SetDefault("configuration.scoreThreshold", 0.3)
		v.SetDefault("configuration.tokenSpendThresh", 1500000)
		v.SetDefault("configuration.lruLimit", 2048)
		v.SetDefault("configuration.synthesisBiasVector", 0.7)
		v.SetDefault("configuration.synthesisBiasSynergy", 0.3)

		if err := os.WriteFile(configPath, []byte("mcpServers: {}\nconfiguration:\n  squeezeLevel: 3\n  logLevel: DEBUG\n  scoreThreshold: 0.3\n  tokenSpendThresh: 1500000\n  lruLimit: 2048\n  synthesisBiasVector: 0.7\n  synthesisBiasSynergy: 0.3\n"), 0644); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
		// Retry read after create
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read created config: %w", err)
		}
	}

	cfg, err := LoadFromViper(v)
	if err != nil {
		return nil, err
	}
	cfg.v = v
	cfg.ConfigPath = configPath
	cfg.Name = Name
	cfg.Version = version
	cfg.DBPath = dbPath
	cfg.LogPath = DefaultLogPath()

	// Default minification is Level 3 (800 tokens)
	if cfg.MaxResponseTokens == 0 {
		cfg.MaxResponseTokens = 800
	}

	return cfg, nil
}

// FoldedString is undocumented but satisfies standard structural requirements.
type FoldedString string

// MarshalYAML forces the string to be encoded as a Literal Block Scalar (|)
// which explicitly preserves our manual word wrapping at 120 characters.
func (s FoldedString) MarshalYAML() (any, error) {
	words := strings.Fields(string(s))
	if len(words) == 0 {
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: string(s),
			Style: yaml.LiteralStyle,
		}, nil
	}

	var sb strings.Builder
	currentLen := 0
	for i, word := range words {
		if i > 0 {
			if currentLen+1+len(word) > 120 {
				sb.WriteString("\n")
				currentLen = 0
			} else {
				sb.WriteString(" ")
				currentLen++
			}
		}
		sb.WriteString(word)
		currentLen += len(word)
	}

	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: sb.String(),
		Style: yaml.LiteralStyle,
	}, nil
}

// NativeTool represents the structure of tools defined in the static config
type NativeTool struct {
	Name        string       `yaml:"name" json:"name"`
	Description FoldedString `yaml:"description" json:"description"`
	Category    string       `yaml:"category,omitempty" json:"category,omitzero"`
	InputSchema any          `yaml:"inputSchema" json:"inputSchema"`
	IsNative    bool         `yaml:"is_native,omitempty" json:"is_native,omitzero"`
}

// SaveConfiguration writes ONLY the configuration block to the config file.
// This is the ONLY code path that should modify user-facing configuration values
// (logLevel, squeezeLevel, scoreThreshold). Called exclusively when the user
// explicitly requests a configuration change.
func (c *Config) SaveConfiguration() error {
	path := c.ConfigPath

	var dynamicConfig map[string]any
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &dynamicConfig)
	}
	if dynamicConfig == nil {
		dynamicConfig = make(map[string]any)
	}

	// Build the configuration block from current runtime state
	c.mu.RLock()
	cfgBlock := map[string]any{
		"logFormat":            c.LogFormat,
		"logLevel":             c.LogLevel,
		"mcpLogLevel":          c.MCPLogLevel,
		"squeezeLevel":         c.SqueezeLevelState,
		"validateProxyCalls":   c.ValidateProxyCalls,
		"squeezeBypass":        c.SqueezeBypass,
		"ringBufferTargets":    c.RingBufferTargets,
		"pinnedServers":        c.PinnedServers,
		"tokenSpendThresh":     c.TokenSpendThresh,
		"lruLimit":             c.LRULimit,
		"synthesisBiasVector":  c.SynthesisBiasVector,
		"synthesisBiasSynergy": c.SynthesisBiasSynergy,
		"synthesisBiasRole":    c.SynthesisBiasRole,
		"scoreFusionAlpha":     c.ScoreFusionAlpha,
	}
	if c.ScoreThreshold != 0 {
		cfgBlock["scoreThreshold"] = c.ScoreThreshold
	}

	// Persist the Interactive LLM configuration data
	if c.Intelligence.Provider != "" {
		intelBlock := map[string]any{
			"provider": c.Intelligence.Provider,
			"model":    c.Intelligence.Model,
			"api_key":  c.Intelligence.APIKey,
		}
		if len(c.Intelligence.FallbackModels) > 0 {
			intelBlock["fallback_models"] = c.Intelligence.FallbackModels
		}
		if c.Intelligence.RetryCount > 0 {
			intelBlock["retry_count"] = c.Intelligence.RetryCount
		}
		if c.Intelligence.RetryDelay > 0 {
			intelBlock["retry_delay_seconds"] = c.Intelligence.RetryDelay
		}
		if c.Intelligence.TimeoutSeconds > 0 {
			intelBlock["timeout_seconds"] = c.Intelligence.TimeoutSeconds
		}

		// Embedding Engine fields
		if c.Intelligence.EmbeddingProvider != "" {
			intelBlock["embedding_provider"] = c.Intelligence.EmbeddingProvider
		}
		if c.Intelligence.EmbeddingModel != "" {
			intelBlock["embedding_model"] = c.Intelligence.EmbeddingModel
		}
		if c.Intelligence.EmbeddingAPIKey != "" {
			intelBlock["embedding_api_key"] = c.Intelligence.EmbeddingAPIKey
		}
		if c.Intelligence.EmbeddedDimensionality > 0 {
			intelBlock["embedded_dimensionality"] = c.Intelligence.EmbeddedDimensionality
		}
		if c.Intelligence.EmbeddingAPIURL != "" {
			intelBlock["embedding_api_url"] = c.Intelligence.EmbeddingAPIURL
		}

		cfgBlock["intelligence"] = intelBlock
	}
	c.mu.RUnlock()

	dynamicConfig["configuration"] = cfgBlock

	data, err := yaml.Marshal(dynamicConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config to yaml: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	// Post-write Viper sync
	c.mu.RLock()
	v := c.v
	c.mu.RUnlock()
	if v != nil {
		_ = v.ReadInConfig()
	}

	return nil
}

// UpdateConfigValue updates a single configuration value in a thread-safe manner.
// Supported keys: logLevel, mcpLogLevel, squeezeLevel, scoreThreshold, validateProxyCalls, pinnedServers, squeezeBypass, ringBufferTargets, lruLimit.
// Returns the old value (for logging) and any validation error.
func (c *Config) UpdateConfigValue(key, value string) (oldValue string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch key {
	case "logFormat":
		oldValue = c.LogFormat
		newFormat := strings.ToLower(value)
		if newFormat != "json" && newFormat != "text" {
			return "", fmt.Errorf("logFormat must be 'json' or 'text'")
		}
		c.LogFormat = newFormat
	case "logLevel":
		oldValue = c.LogLevel
		c.LogLevel = strings.ToUpper(value)
	case "mcpLogLevel":
		oldValue = c.MCPLogLevel
		c.MCPLogLevel = strings.ToUpper(value)
	case "squeezeLevel":
		lvl, parseErr := strconv.Atoi(value)
		if parseErr != nil || lvl < 0 || lvl > 5 {
			return "", fmt.Errorf("squeezeLevel must be an integer 0-5, got: %s", value)
		}
		if c.SqueezeLevelState != nil {
			oldValue = strconv.Itoa(*c.SqueezeLevelState)
		}
		c.SqueezeLevelState = &lvl

		// Apply Preset Override System at runtime (mirrors LoadFromViper boot logic)
		switch lvl {
		case 0:
			c.NoOptimize = true
			c.MaxResponseTokens = 0
		case 1:
			c.NoOptimize = false
			c.MaxResponseTokens = 600
		case 2:
			c.NoOptimize = false
			c.MaxResponseTokens = 1000
		case 3:
			c.NoOptimize = false
			c.MaxResponseTokens = 1400
		case 4:
			c.NoOptimize = false
			c.MaxResponseTokens = 1800
		case 5:
			c.NoOptimize = false
			c.MaxResponseTokens = 2400
		}
	case "scoreThreshold":
		val, parseErr := strconv.ParseFloat(value, 64)
		if parseErr != nil || val < 0 || val > 1 {
			return "", fmt.Errorf("scoreThreshold must be a float 0.0-1.0, got: %s", value)
		}
		oldValue = strconv.FormatFloat(c.ScoreThreshold, 'f', -1, 64)
		c.ScoreThreshold = val
	case "validateProxyCalls":
		oldValue = strconv.FormatBool(c.ValidateProxyCalls)
		switch strings.ToLower(value) {
		case "true", "1", "yes", "on":
			c.ValidateProxyCalls = true
		case "false", "0", "no", "off":
			c.ValidateProxyCalls = false
		default:
			return "", fmt.Errorf("validateProxyCalls must be true/false, got: %s", value)
		}
	case "pinnedServers":
		oldValue = strings.Join(c.PinnedServers, " ")
		value = strings.TrimSpace(value)
		if value == "" {
			c.PinnedServers = nil
		} else {
			c.PinnedServers = strings.Fields(value)
		}
	case "squeezeBypass":
		oldValue = strings.Join(c.SqueezeBypass, " ")
		value = strings.TrimSpace(value)
		if value == "" {
			c.SqueezeBypass = nil
		} else {
			c.SqueezeBypass = strings.Fields(value)
		}
	case "ringBufferTargets":
		oldValue = strings.Join(c.RingBufferTargets, " ")
		value = strings.TrimSpace(value)
		if value == "" {
			c.RingBufferTargets = nil
		} else {
			c.RingBufferTargets = strings.Fields(value)
		}
	case "tokenSpendThresh":
		val, parseErr := strconv.Atoi(value)
		if parseErr != nil || val < 0 {
			return "", fmt.Errorf("tokenSpendThresh must be a positive integer, got: %s", value)
		}
		oldValue = strconv.Itoa(c.TokenSpendThresh)
		c.TokenSpendThresh = val
	case "lruLimit":
		val, parseErr := strconv.Atoi(value)
		if parseErr != nil || val <= 0 {
			return "", fmt.Errorf("lruLimit must be a positive integer, got: %s", value)
		}
		oldValue = strconv.Itoa(c.LRULimit)
		c.LRULimit = val
	default:
		return "", fmt.Errorf("unsupported config key: %s (supported: logLevel, squeezeLevel, scoreThreshold, validateProxyCalls, pinnedServers, squeezeBypass, ringBufferTargets, tokenSpendThresh, lruLimit)", key)
	}
	return oldValue, nil
}

// LoadFromViper extracts configuration from a Viper instance and loads
// managed servers from the native servers.yaml registry.
func LoadFromViper(v *viper.Viper) (*Config, error) {
	var ide IDEConfig

	// First, let viper load whatever it has
	if err := v.Unmarshal(&ide); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Dynamic Discovery: Look for actual IDE mcp_config.json for configuration block only.
	var ideConfigPath string
	if path := v.ConfigFileUsed(); path != "" && strings.HasSuffix(path, ".json") {
		ideConfigPath = path
	} else {
		ideConfigPath, _ = DiscoverIDEConfig()
	}

	if data, err := os.ReadFile(ideConfigPath); err == nil {
		var raw IDEConfig
		if err := json.Unmarshal(data, &raw); err == nil {
			ide.McpServers = raw.McpServers
			if raw.Configuration.MaxResponseTokens != 0 {
				ide.Configuration.MaxResponseTokens = raw.Configuration.MaxResponseTokens
			}
			if raw.Configuration.SqueezeLevel != nil {
				ide.Configuration.SqueezeLevel = raw.Configuration.SqueezeLevel
			}
			if len(raw.Configuration.SqueezeBypass) > 0 {
				ide.Configuration.SqueezeBypass = append([]string{}, raw.Configuration.SqueezeBypass...)
			}
			if len(raw.Configuration.RingBufferTargets) > 0 {
				ide.Configuration.RingBufferTargets = append([]string{}, raw.Configuration.RingBufferTargets...)
			}
		}
	}

	// 🛡️ NATIVE REGISTRY: Load managed servers from servers.yaml (orchestrator-owned).
	// Auto-migration: if servers.yaml doesn't exist, extract from IDE config and write it.
	managed, err := LoadManagedServers()
	if err != nil {
		slog.Info("config: servers.yaml not found, migrating from IDE config", "error", err)
		managed = extractManaged(&ide)
		if len(managed) > 0 {
			if saveErr := SaveManagedServers(managed); saveErr != nil {
				slog.Warn("config: failed to write servers.yaml during migration", "error", saveErr)
			} else {
				slog.Info("config: auto-migrated servers to servers.yaml", "count", len(managed))
			}
		}
	}

	cfg := &Config{
		ManagedServers:       managed,
		ideConfig:            &ide,
		MaxResponseTokens:    ide.Configuration.MaxResponseTokens,
		SqueezeLevelState:    ide.Configuration.SqueezeLevel,
		ScoreThreshold:       ide.Configuration.ScoreThreshold,
		SqueezeBypass:        ide.Configuration.SqueezeBypass,
		RingBufferTargets:    ide.Configuration.RingBufferTargets,
		PinnedServers:        ide.Configuration.PinnedServers,
		LogFormat:            ide.Configuration.LogFormat,
		LogLevel:             ide.Configuration.LogLevel,
		MCPLogLevel:          ide.Configuration.MCPLogLevel,
		TokenSpendThresh:     ide.Configuration.TokenSpendThresh,
		LRULimit:             ide.Configuration.LRULimit,
		SynthesisBiasVector:  ide.Configuration.SynthesisBiasVector,
		SynthesisBiasSynergy: ide.Configuration.SynthesisBiasSynergy,
		SynthesisBiasRole:    ide.Configuration.SynthesisBiasRole,
		ScoreFusionAlpha:     ide.Configuration.ScoreFusionAlpha,
		Intelligence:         ide.Configuration.Intelligence,
	}

	normalizeRRFBiases(cfg)

	if cfg.TokenSpendThresh == 0 {
		cfg.TokenSpendThresh = 1500000
	}
	if cfg.LRULimit == 0 {
		cfg.LRULimit = 2048
	}

	// 🛡️ INTELLIGENCE DEFAULTS: Apply safe defaults for hydrator retry/timeout
	if cfg.Intelligence.RetryCount <= 0 {
		cfg.Intelligence.RetryCount = 2
	}
	if cfg.Intelligence.RetryDelay <= 0 {
		cfg.Intelligence.RetryDelay = 5
	}
	if cfg.Intelligence.TimeoutSeconds <= 0 {
		cfg.Intelligence.TimeoutSeconds = 120
	}

	// 🛡️ EMBEDDING DEFAULTS: Resolve embedding provider/model from parent if omitted
	if cfg.Intelligence.EmbeddingProvider == "" && cfg.Intelligence.Provider != "" {
		switch cfg.Intelligence.Provider {
		case "gemini", "openai":
			cfg.Intelligence.EmbeddingProvider = cfg.Intelligence.Provider
		case "anthropic":
			// Anthropic has no embedding API; auto-detect from env
			if os.Getenv("GEMINI_API_KEY") != "" {
				cfg.Intelligence.EmbeddingProvider = "gemini"
			} else if os.Getenv("OPENAI_API_KEY") != "" {
				cfg.Intelligence.EmbeddingProvider = "openai"
			} else if os.Getenv("VOYAGE_API_KEY") != "" {
				cfg.Intelligence.EmbeddingProvider = "voyage"
			}
		}
	}
	if cfg.Intelligence.EmbeddingModel == "" {
		switch cfg.Intelligence.EmbeddingProvider {
		case "gemini":
			cfg.Intelligence.EmbeddingModel = "gemini-embedding-2-preview"
		case "openai":
			cfg.Intelligence.EmbeddingModel = "text-embedding-3-small"
		case "voyage":
			cfg.Intelligence.EmbeddingModel = "voyage-code-3"
		case "ollama":
			cfg.Intelligence.EmbeddingModel = "granite-embedding:30m"
		}
	}
	if cfg.Intelligence.EmbeddingAPIKey == "" {
		if cfg.Intelligence.EmbeddingProvider == cfg.Intelligence.Provider {
			cfg.Intelligence.EmbeddingAPIKey = cfg.Intelligence.APIKey
		} else {
			// Cross-provider: check env for the embedding provider's key
			switch cfg.Intelligence.EmbeddingProvider {
			case "gemini":
				cfg.Intelligence.EmbeddingAPIKey = os.Getenv("GEMINI_API_KEY")
			case "openai":
				cfg.Intelligence.EmbeddingAPIKey = os.Getenv("OPENAI_API_KEY")
			case "voyage":
				cfg.Intelligence.EmbeddingAPIKey = os.Getenv("VOYAGE_API_KEY")
			}
		}
	}
	if cfg.Intelligence.EmbeddedDimensionality <= 0 {
		switch cfg.Intelligence.EmbeddingProvider {
		case "gemini", "openai", "voyage":
			cfg.Intelligence.EmbeddedDimensionality = 768
		default:
			// Ollama and all future local/self-hosted providers default to 384.
			// Users may override to 768 for larger models (e.g., granite-embedding:278m)
			// by setting embedded_dimensionality explicitly in config.yaml.
			cfg.Intelligence.EmbeddedDimensionality = 384
		}
	}
	if cfg.Intelligence.EmbeddingAPIURL == "" && cfg.Intelligence.EmbeddingProvider == "ollama" {
		cfg.Intelligence.EmbeddingAPIURL = "http://localhost:11434"
	}

	if len(cfg.SqueezeBypass) > 0 {
		slog.Info("config: squeeze bypass loaded", "servers", cfg.SqueezeBypass)
	}
	if len(cfg.RingBufferTargets) > 0 {
		slog.Info("config: ring buffer targets loaded", "targets", cfg.RingBufferTargets)
	}

	if ide.Configuration.ValidateProxyCalls != nil {
		cfg.ValidateProxyCalls = *ide.Configuration.ValidateProxyCalls
	} else {
		cfg.ValidateProxyCalls = true
	}

	if cfg.ScoreThreshold == 0.0 {
		cfg.ScoreThreshold = 0.3
	}

	// Preset Override System
	if ide.Configuration.SqueezeLevel != nil {
		switch *ide.Configuration.SqueezeLevel {
		case 1:
			cfg.MaxResponseTokens = 600
		case 2:
			cfg.MaxResponseTokens = 1000
		case 3:
			cfg.MaxResponseTokens = 1400
		case 4:
			cfg.MaxResponseTokens = 1800
		case 5:
			cfg.MaxResponseTokens = 2400
		case 0:
			cfg.NoOptimize = true
			cfg.MaxResponseTokens = 0
		default:
			cfg.MaxResponseTokens = 1400
		}
	} else {
		// If explicitly omitted, respect viper default level 3 (which was set in SetDefault) or default to 800
		cfg.MaxResponseTokens = 800
	}

	return cfg, nil
}

// Load parses the IDE config file and extracts managed servers (Legacy/CLI wrapper)
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	cfg, err := LoadFromViper(v)
	if err != nil {
		return nil, err
	}
	cfg.v = v
	cfg.ConfigPath = path
	return cfg, nil
}

// normalizeRRFBiases mathematically enforces the 3-way constraint where
// SynthesisBiasVector + SynthesisBiasSynergy + SynthesisBiasRole must equal 1.0.
// Defaults: Engine=0.5, Synergy=0.2, Role=0.3. ScoreFusionAlpha defaults to 0.6.
func normalizeRRFBiases(cfg *Config) {
	if cfg.SynthesisBiasVector <= 0 && cfg.SynthesisBiasSynergy <= 0 && cfg.SynthesisBiasRole <= 0 {
		cfg.SynthesisBiasVector = 0.5
		cfg.SynthesisBiasSynergy = 0.2
		cfg.SynthesisBiasRole = 0.3
	}
	if cfg.SynthesisBiasRole <= 0 {
		cfg.SynthesisBiasRole = 0.3
	}

	total := cfg.SynthesisBiasVector + cfg.SynthesisBiasSynergy + cfg.SynthesisBiasRole
	if total != 1.0 && total > 0 {
		cfg.SynthesisBiasVector = cfg.SynthesisBiasVector / total
		cfg.SynthesisBiasSynergy = cfg.SynthesisBiasSynergy / total
		cfg.SynthesisBiasRole = cfg.SynthesisBiasRole / total
	}

	if cfg.ScoreFusionAlpha <= 0 {
		cfg.ScoreFusionAlpha = 0.6
	}
	if cfg.ScoreFusionAlpha > 1.0 {
		cfg.ScoreFusionAlpha = 1.0
	}
}

// Reload re-reads the config file and returns a new Config
func (c *Config) Reload() (*Config, error) {
	if c.v != nil {
		if err := c.v.ReadInConfig(); err != nil {
			return nil, err
		}
		cfg, err := LoadFromViper(c.v)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}
	cfg, err := Load(c.ConfigPath)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// Viper returns the underlying viper instance
func (c *Config) Viper() *viper.Viper {
	return c.v
}

// GetManagedServerNames returns just the names of managed servers
func (c *Config) GetManagedServerNames() map[string]bool {
	names := make(map[string]bool, len(c.ManagedServers))
	for _, s := range c.ManagedServers {
		names[s.Name] = true
	}
	return names
}

// extractManaged filters IDE config for disabled: true entries, excluding self.
// Used only during auto-migration to seed servers.yaml from legacy IDE config.
func extractManaged(ide *IDEConfig) []ServerConfig {
	var servers []ServerConfig
	for name, entry := range ide.McpServers {
		if name == SelfName {
			continue // never manage ourselves
		}
		if !entry.Disabled {
			continue // IDE manages enabled servers
		}
		servers = append(servers, ServerConfig{
			Name:          name,
			Command:       entry.Command,
			Args:          entry.Args,
			Env:           entry.Env,
			DisabledTools: entry.DisabledTools,
		})
	}
	return servers
}

// serversYAML is the on-disk format for the native server registry.
type serversYAML struct {
	Servers []serverEntry `yaml:"servers"`
}

type serverEntry struct {
	Name          string            `yaml:"name"`
	Command       string            `yaml:"command"`
	Args          []string          `yaml:"args,omitempty"`
	Env           map[string]string `yaml:"env,omitempty"`
	DisabledTools []string          `yaml:"disabled_tools,omitempty"`
	MemoryLimitMB int               `yaml:"memory_limit_mb,omitempty"`
	GoMemLimitMB  int               `yaml:"gomemlimit_mb,omitempty"`
	MaxCPULimit   int               `yaml:"max_cpu_limit,omitempty"`
	DeferredBoot  bool              `yaml:"deferred_boot,omitempty"`
}

// LoadManagedServers reads the native server registry from servers.yaml.
func LoadManagedServers() ([]ServerConfig, error) {
	path := filepath.Join(DefaultConfigDir(), ServersConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("servers.yaml not found: %w", err)
	}

	var reg serversYAML
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("servers.yaml parse error: %w", err)
	}

	servers := make([]ServerConfig, 0, len(reg.Servers))
	for _, e := range reg.Servers {
		servers = append(servers, ServerConfig{
			Name:          e.Name,
			Command:       e.Command,
			Args:          e.Args,
			Env:           e.Env,
			DisabledTools: e.DisabledTools,
			MemoryLimitMB: e.MemoryLimitMB,
			GoMemLimitMB:  e.GoMemLimitMB,
			MaxCPULimit:   e.MaxCPULimit,
			DeferredBoot:  e.DeferredBoot,
		})
	}

	slog.Info("config: loaded servers from native registry", "count", len(servers), "path", path)
	return servers, nil
}

// SaveManagedServers writes the native server registry to servers.yaml.
func SaveManagedServers(servers []ServerConfig) error {
	path := filepath.Join(DefaultConfigDir(), ServersConfigFile)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	reg := serversYAML{Servers: make([]serverEntry, 0, len(servers))}
	for _, sc := range servers {
		reg.Servers = append(reg.Servers, serverEntry{
			Name:          sc.Name,
			Command:       sc.Command,
			Args:          sc.Args,
			Env:           sc.Env,
			DisabledTools: sc.DisabledTools,
			MemoryLimitMB: sc.MemoryLimitMB,
			GoMemLimitMB:  sc.GoMemLimitMB,
			MaxCPULimit:   sc.MaxCPULimit,
			DeferredBoot:  sc.DeferredBoot,
		})
	}

	data, err := yaml.Marshal(&reg)
	if err != nil {
		return fmt.Errorf("failed to marshal servers.yaml: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write servers.yaml: %w", err)
	}

	slog.Info("config: saved native server registry", "count", len(servers), "path", path)
	return nil
}
