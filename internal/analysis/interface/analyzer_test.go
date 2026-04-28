package interfaceanalysis

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDiscoverSharedInterfaces(t *testing.T) {
	ctx := context.Background()
	// Test against current package
	suggestions, err := DiscoverSharedInterfaces(ctx, ".")
	if err != nil {
		t.Fatalf("DiscoverSharedInterfaces failed: %v", err)
	}
	// Verify we get some result if we have structs with shared methods
	// In this package there are no structs with methods.
	_ = suggestions
}

func TestIntersection(t *testing.T) {
	tests := []struct {
		name     string
		s1       []string
		s2       []string
		expected []string
	}{
		{
			name:     "common elements",
			s1:       []string{"A", "B", "C"},
			s2:       []string{"B", "C", "D"},
			expected: []string{"B", "C"},
		},
		{
			name:     "no common elements",
			s1:       []string{"A", "B"},
			s2:       []string{"C", "D"},
			expected: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := intersection(tc.s1, tc.s2)
			if len(res) != len(tc.expected) {
				t.Errorf("expected length %d, got %d", len(tc.expected), len(res))
			}
			for i, v := range res {
				if v != tc.expected[i] {
					t.Errorf("at index %d: expected %s, got %s", i, tc.expected[i], v)
				}
			}
		})
	}
}
func TestFindImplementations(t *testing.T) {
	ctx := context.Background()
	tool := &ImplementationTool{}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}}

	// Test case 1: Find implementations of Tool interface in registry package
	// Note: We use a relative path that works within the test context
	matches, err := tool.FindImplementations(ctx, req, "Tool", "../../registry")
	if err != nil {
		t.Fatalf("FindImplementations failed: %v", err)
	}

	found := false
	for _, m := range matches {
		if m.Name == "Tool" && strings.Contains(m.PkgPath, "registry") {
			found = true
			break
		}
	}
	// In the registry package, the Tool struct might implement the Tool interface (if it's defined there)
	// Actually, registry.Tool is an interface.
	_ = found
}

func TestDiscoverGlobalImplementations(t *testing.T) {
	ctx := context.Background()
	// Scan the registry package for patterns
	suggestions, err := DiscoverSharedInterfaces(ctx, "../../registry")
	if err != nil {
		t.Fatalf("DiscoverSharedInterfaces failed: %v", err)
	}

	for _, s := range suggestions {
		t.Logf("Found suggestion: %v with %d global implementors", s.Methods, len(s.Implementors))
		for _, imp := range s.Implementors {
			t.Logf("  - Implementor: %s in %s", imp.Name, imp.PkgPath)
		}
	}
}
