package main

import (
	"fmt"
	"os"
)

// Version is the current version of the Sequential Thinking MCP server.
var Version = "0.0.0-dev"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-sequential-thinking version %s\n", Version)
}
