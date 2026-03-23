package metrics

import (
	"context"
	"testing"
)

func TestCalculateComplexity(t *testing.T) {
	// Using "." for relative package path
	result, err := CalculateComplexity(context.Background(), ".")
	if err != nil {
		t.Fatalf("failed to calculate complexity: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	foundCalculateComplexity := false
	for name := range result.Functions {
		if name == "CalculateComplexity" {
			foundCalculateComplexity = true
			break
		}
	}
	if !foundCalculateComplexity {
		t.Errorf("expected CalculateComplexity in function complexity list; got %v", result.Functions)
	}
}
