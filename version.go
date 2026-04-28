package main

import (
	"fmt"
	"os"
)

// Version is the current version of the Brainstorm MCP server.
var Version = "0.0.0-dev"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-brainstorm version %s\n", Version)
}
