package main

import (
	"fmt"
	"os"
)

// Version is the current version of the MagicTools MCP server.
var Version = "v4.2.9"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-magictools version %s\n", Version)
}
