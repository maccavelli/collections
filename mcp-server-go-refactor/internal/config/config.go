// Package config provides functionality for the config subsystem.
package config

import (
	"os"
	"strings"
)

const (
	// Project Identity
	Name     = "mcp-server-go-refactor"
	Platform = "Go Refactor"

	// System Limits
	LogBufferLimit  = 1024 * 1024 // 1MB
	LogTrimTarget   = 512 * 1024  // 512KB remains after trim
	DefaultLogLines = 25

	// Complexity Analysis
	CyclomaticComplexityTarget = 10
	CognitiveComplexityTarget  = 15
)

// ResolveAPIURLs canonicalizes the comma-separated MCP_API_URL environment variable.
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

// IsOrchestrated evaluates the payload's session ID to determine if the tool
// is running inside an active orchestrator pipeline.
func IsOrchestrated(sessionID string) bool {
	return strings.TrimSpace(sessionID) != ""
}
