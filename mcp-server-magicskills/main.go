package main

import (
	"os"

	"mcp-server-magicskills/cmd"
)

func main() {
	// Defense-in-depth: Unmanaged Standalone Fallbacks
	if _, exists := os.LookupEnv("GOMEMLIMIT"); !exists {
		os.Setenv("GOMEMLIMIT", "1024MiB")
	}
	if _, exists := os.LookupEnv("GOMAXPROCS"); !exists {
		os.Setenv("GOMAXPROCS", "2")
	}

	cmd.Execute(Version)
}
