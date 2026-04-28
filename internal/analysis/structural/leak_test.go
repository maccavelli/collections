package structural

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestLeakAnalysis(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "unguarded select",
			src: `package main
func f() {
	var ch chan int
	select {
	case ch <- 1:
	}
}`,
			want: 1,
		},
		{
			name: "guarded select default",
			src: `package main
func f() {
	var ch chan int
	select {
	case ch <- 1:
	default:
	}
}`,
			want: 0,
		},
		{
			name: "guarded select done",
			src: `package main
import "context"
func f(ctx context.Context) {
	var ch chan int
	select {
	case ch <- 1:
	case <-ctx.Done():
	}
}`,
			want: 0,
		},
		{
			name: "naked go",
			src: `package main
func f() {
	go func() {
		for {}
	}()
}`,
			want: 1,
		},
		{
			name: "go with context argument",
			src: `package main
import "context"
func f(ctx context.Context) {
	go func(c context.Context) {
		<-c.Done()
	}(ctx)
}`,
			want: 0,
		},
		{
			name: "clean file",
			src: `package main
func f() {
	println("hello")
}`,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, "", tt.src, 0)
			if err != nil {
				t.Fatalf("ParseFile error: %v", err)
			}

			i := &Inspector{}
			var gaps int

			ast.Inspect(node, func(n ast.Node) bool {
				switch fn := n.(type) {
				case *ast.SelectStmt:
					res := i.checkUnguardedSelect(fset, fn, "test.go")
					gaps += len(res)
				case *ast.GoStmt:
					res := i.checkNakedGoroutine(fset, fn, "test.go")
					gaps += len(res)
				}
				return true
			})

			if gaps != tt.want {
				t.Errorf("got %d gaps, want %d", gaps, tt.want)
			}
		})
	}
}
