package main

import "fmt"

// Version is overwritten by build flags
var Version = "3.1.2"

func printVersion() {
	fmt.Printf("mcp-server-duckduckgo version %s\n", Version)
}
