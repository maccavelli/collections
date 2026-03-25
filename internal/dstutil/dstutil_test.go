package dstutil

import (
	"bytes"
	"go/parser"
	"go/token"
	"testing"
)

func TestToDST(t *testing.T) {
	fset := token.NewFileSet()
	src := `package main
// Important comment
func main() {}`
	f, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	dstFile, err := ToDST(fset, f)
	if err != nil {
		t.Fatalf("ToDST failed: %v", err)
	}

	if dstFile.Name.Name != "main" {
		t.Errorf("expected package main, got %s", dstFile.Name.Name)
	}

	// Verify comments are preserved
	data, err := WriteFile(dstFile)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if !bytes.Contains(data, []byte("// Important comment")) {
		t.Errorf("expected comment preserved, output: %s", string(data))
	}
}
