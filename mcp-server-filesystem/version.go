package main

import (
	"fmt"
	"os"
)

// Version is the current version of the Filesystem MCP server.
var Version = "0.0.0-dev"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-filesystem version %s\n", Version)
}
