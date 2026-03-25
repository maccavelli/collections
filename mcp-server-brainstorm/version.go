package main

import "fmt"

// Version is the current version of the Brainstorm MCP server.
var Version = "3.1.2"

func printVersion() {
	fmt.Printf("mcp-server-brainstorm version %s\n", Version)
}
