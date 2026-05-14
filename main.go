package main

import (
	"mcp-server-socratic-thinker/internal/cmd"
)



func main() {
	cmd.Version = Version
	cmd.Execute()
}
