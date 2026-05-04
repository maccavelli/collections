// Package main provides functionality for the main subsystem.
package main

import (
	"fmt"
	"os"
)

// Version is the current version of the Go Refactor MCP server.
var Version = "v4.3.2"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-go-refactor version %s\n", Version)
}
