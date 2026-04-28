package main

import (
	"fmt"
	"os"
)

// Version is the current version of the MagicSkills MCP server.
var Version = "v4.2.9"

func printVersion() {
	fmt.Fprintf(os.Stderr, "mcp-server-magicskills version %s\n", Version)
}
