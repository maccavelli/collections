// Package main provides functionality for the main subsystem.
package main

import (
	"fmt"
	"os"
)

// Version is the current version of the Brainstorm MCP server.
var Version = "v4.2.12"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-brainstorm version %s\n", Version)
}
