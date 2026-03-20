package main

import "fmt"

// Version is the current version of the Brainstorm MCP server.
var Version = "0.1.0"

func printVersion() {
	fmt.Printf("mcp-server-brainstorm version %s\n", Version)
}
