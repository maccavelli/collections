// Package config provides functionality for the config subsystem.
package config

import (
	"os"
	"strings"
)

const (
	// Project Identity
	Name     = "mcp-server-brainstorm"
	Platform = "Brainstorm"

	// System Limits
	LogBufferLimit  = 1024 * 1024 // 1MB
	LogTrimTarget   = 512 * 1024  // 512KB remains after trim
	DefaultLogLines = 25
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

// IsOrchestratorOwned checks if the Aporia Engine should interact with the swarm.
func IsOrchestratorOwned() bool {
	return os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
}
