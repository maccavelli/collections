package interfaceanalysis

import (
	"context"
	"testing"
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
