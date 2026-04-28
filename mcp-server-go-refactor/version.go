package main

import (
	"fmt"
	"os"
)

// Version is the current version of the Go Refactor MCP server.
var Version = "v4.2.6"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-go-refactor version %s\n", Version)
}
