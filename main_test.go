package main

import (
	"os"
	"testing"
)

func TestMainCmd(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	
	os.Args = []string{"magicdev", "--help"}
	main()
}
