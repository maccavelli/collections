package config

const (
	// Project Identity
	Name     = "mcp-server-brainstorm"
	Platform = "Brainstorm"

	// System Limits
	LogBufferLimit = 1024 * 1024 // 1MB
	LogTrimTarget  = 512 * 1024  // 512KB remains after trim
	DefaultLogLines = 25
)
