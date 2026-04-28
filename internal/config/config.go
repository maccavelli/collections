package config

// Name is the MCP server identifier used in protocol handshakes.
const Name = "mcp-server-filesystem"

// Platform is the human-readable name shown to clients.
const Platform = "Secure Filesystem"

// Log buffer limits.
const (
	LogBufferLimit  = 1024 * 1024 // 1MB max log buffer
	LogTrimTarget   = 512 * 1024  // 512KB trim target
	DefaultLogLines = 25          // Default lines returned by get_internal_logs
)

// Safety limits.
const (
	MaxReadFileSize  = 50 * 1024 * 1024 // 50MB max file read
	MaxTreeDepth     = 20               // Max directory tree recursion depth
	MaxSearchResults = 1000             // Max results from search_files
)
