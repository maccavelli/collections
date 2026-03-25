package config

import (
	"os"
	"path/filepath"
)

const (
	Name          = "mcp-server-recall"
	DefaultVersion = "0.2.0"
	DefaultDBPath = ".mcp_recall"
	EnvDBPath     = "MCP_RECALL_DB_PATH"
)

// Config holds the application configuration.
type Config struct {
	Name    string
	Version string
	DBPath  string
}

// New() initializes a new Config with defaults and environment overrides.
func New() *Config {
	dbPath := os.Getenv(EnvDBPath)
	if dbPath == "" {
		dbPath = DefaultDBPath
	}

	// Ensure path is absolute if specified as a relative local directory
	// but maintain simple local default if possible.
	return &Config{
		Name:    Name,
		Version: DefaultVersion,
		DBPath:  dbPath,
	}
}

// GetDBPath returns the absolute path to the database directory.
func (c *Config) GetDBPath() string {
	if filepath.IsAbs(c.DBPath) {
		return c.DBPath
	}
	// Defaults to local directory if not absolute.
	return c.DBPath
}
