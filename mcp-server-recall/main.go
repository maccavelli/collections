package main

import (
	"fmt"
	"mcp-server-recall/cmd"
	"os"
)

func main() {
	// Defense-in-depth: Unmanaged Standalone Fallbacks
	if _, exists := os.LookupEnv("GOMEMLIMIT"); !exists {
		os.Setenv("GOMEMLIMIT", "1024MiB")
	}
	if _, exists := os.LookupEnv("GOMAXPROCS"); !exists {
		os.Setenv("GOMAXPROCS", "2")
	}
	f, err := os.OpenFile("/tmp/mcp-server-recall/crash.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err == nil {
		fmt.Fprintf(f, "MAIN STARTING args: %v\n", os.Args)
		defer func() {
			fmt.Fprintf(f, "MAIN EXITED\n")
			f.Close()
		}()
	}
	cmd.Execute(Version)
}
