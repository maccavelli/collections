package main

import "fmt"

// Version is overwritten by build flags
var Version = "3.2.0"

func printVersion() {
	fmt.Printf("mcp-server-duckduckgo version %s\n", Version)
}
