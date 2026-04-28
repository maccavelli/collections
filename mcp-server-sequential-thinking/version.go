package main

import (
	"fmt"
	"os"
)

// Version is the current version of the Sequential Thinking MCP server.
var Version = "v4.2.6"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-sequential-thinking version %s\n", Version)
}
