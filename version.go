package main

import (
	"fmt"
	"os"
)

// Version is the current version of the Recall MCP server.
var Version = "v4.3.2"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-recall version %s\n", Version)
}
