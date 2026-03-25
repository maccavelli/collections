package config

const (
	// Project Identity
	Name     = "mcp-server-go-refactor"
	Platform = "Go Refactor"

	// System Limits
	LogBufferLimit = 1024 * 1024 // 1MB
	LogTrimTarget  = 512 * 1024  // 512KB remains after trim
	DefaultLogLines = 25

	// Complexity Analysis
	CyclomaticComplexityTarget = 10
	CognitiveComplexityTarget  = 15
)
