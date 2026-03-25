package dstutil

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
)

// ToDST converts an ast.File into a dst.File, preserving comments as decorations.
func ToDST(fset *token.FileSet, file *ast.File) (*dst.File, error) {
	d := decorator.NewDecorator(fset)
	dstFile, err := d.DecorateFile(file)
	if err != nil {
		return nil, fmt.Errorf("decorate file: %w", err)
	}
	return dstFile, nil
}

// WriteFile formats a DST file as Go source.
func WriteFile(dstFile *dst.File) ([]byte, error) {
	var buf bytes.Buffer
	if err := decorator.Fprint(&buf, dstFile); err != nil {
		return nil, fmt.Errorf("fprint: %w", err)
	}
	return buf.Bytes(), nil
}
