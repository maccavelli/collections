package main

import "fmt"

// Version is overwritten by build flags
var Version = "1.0.1"

func printVersion() {
	fmt.Printf("mcp-server-duckduckgo version %s\n", Version)
}
