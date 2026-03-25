package contextanalysis

import (
	"context"
	"go/ast"
	"testing"
)

func TestAnalyzeContext(t *testing.T) {
	ctx := context.Background()
	// Test against current package
	findings, err := AnalyzeContext(ctx, ".")
	if err != nil {
		t.Fatalf("AnalyzeContext failed: %v", err)
	}
	// Currently it should return 0 findings for itself as it's following context (or maybe not?)
	// But at least it shouldn't crash.
	_ = findings
}

func TestIsContextDeprivedCall(t *testing.T) {
	tests := []struct {
		name     string
		funName  string
		pkgName  string
		expected bool
	}{
		{
			name:     "context.Background()",
			funName:  "Background",
			pkgName:  "context",
			expected: true,
		},
		{
			name:     "context.TODO()",
			funName:  "TODO",
			pkgName:  "context",
			expected: true,
		},
		{
			name:     "other.Background()",
			funName:  "Background",
			pkgName:  "other",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Actually isContextDeprivedCall checks if the FIRST ARG of 'call' is context.Background()
			
			dummyCall := &ast.CallExpr{
				Fun: &ast.Ident{Name: "SomeFunc"},
				Args: []ast.Expr{
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   &ast.Ident{Name: tc.pkgName},
							Sel: &ast.Ident{Name: tc.funName},
						},
					},
				},
			}

			result := isContextDeprivedCall(nil, dummyCall)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}
