package main

import (
	"fmt"
	"os"
)

// Version is the current version of the DuckDuckGo MCP server.
var Version = "v4.3.4"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-duckduckgo version %s\n", Version)
}
