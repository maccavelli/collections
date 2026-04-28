package main

import (
	"fmt"
	"os"
)

// Version is the current version of the Go Refactor MCP server.
var Version = "0.0.0-dev"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-go-refactor version %s\n", Version)
}
