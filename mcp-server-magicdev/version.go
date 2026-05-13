package main

import (
	"fmt"
	"os"
)

// Version is the current version of the MagicDev MCP server.
var Version = "v4.4.4"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-magicdev version %s\n", Version)
}
