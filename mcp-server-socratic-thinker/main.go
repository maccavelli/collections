package main

import (
	"mcp-server-socratic-thinker/internal/cmd"
)

// Version is injected at build time via ldflags.
var Version = "dev"

func main() {
	cmd.Version = Version
	cmd.Execute()
}
