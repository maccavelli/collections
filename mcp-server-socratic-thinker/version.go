package main

import (
	"fmt"
	"os"
)

// Version is the current version of the Socratic Thinker MCP server.
var Version = "v1.0.2"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-socratic-thinker version %s\n", Version)
}
