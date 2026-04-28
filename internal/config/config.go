package config

const (
	Name     = "mcp-server-sequential-thinking"

	LogBufferLimit  = 1024 * 1024 // 1MB max log buffer
	LogTrimTarget   = 512 * 1024  // 512KB trim target
	DefaultLogLines = 25          // Default lines returned by get_internal_logs
)
