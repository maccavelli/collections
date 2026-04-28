package main

import (
	"fmt"
	"os"
)

// Version is the current version of the Filesystem MCP server.
var Version = "v4.2.6"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-filesystem version %s\n", Version)
}
