package structural

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestGenericAnalysis(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "bloated interface CRTP style",
			src: `package main
type Validator[T any, V any] interface {
	Validate(T) V
}`,
			want: 1,
		},
		{
			name: "bloated interface multi field",
			src: `package main
type Node[K comparable, V any] interface {
	Get(K) V
}`,
			want: 1,
		},
		{
			name: "Go 1.26 single self-ref interface",
			src: `package main
type Node[T Node[T]] interface {
	Next() T
}`,
			want: 0,
		},
		{
			name: "non-generic interface",
			src: `package main
type Reader interface {
	Read(p []byte) (n int, err error)
}`,
			want: 0,
		},
		{
			name: "structs should be ignored",
			src: `package main
type Container[T any, V any] struct {
	TVal T
	VVal V
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
				case *ast.TypeSpec:
					if iface, ok := fn.Type.(*ast.InterfaceType); ok {
						res := i.checkInterfaceBloat(fset, fn.Name.Name, fn.TypeParams, iface, "test.go")
						gaps += len(res)
					}
				}
				return true
			})

			if gaps != tt.want {
				t.Errorf("got %d gaps, want %d", gaps, tt.want)
			}
		})
	}
}
