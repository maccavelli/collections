package docgen

import (
	"context"
	"strings"
	"testing"
)

type ExportedTypeMissingDoc struct{}

func ExportedFuncMissingDoc() {}

func TestGenerateDocs(t *testing.T) {
	summary, err := GenerateDocs(context.Background(), ".")
	if err != nil {
		t.Fatalf("failed to generate docs: %v", err)
	}

	if summary == nil {
		t.Fatal("expected summary, got nil")
	}

	foundFunc := false
	foundType := false
	for _, m := range summary.MissingComments {
		if strings.Contains(m, "func ExportedFuncMissingDoc") {
			foundFunc = true
		}
		if strings.Contains(m, "type ExportedTypeMissingDoc") {
			foundType = true
		}
	}

	if !foundFunc {
		t.Errorf("expected ExportedFuncMissingDoc in missing comments")
	}
	if !foundType {
		t.Errorf("expected ExportedTypeMissingDoc in missing comments")
	}
}
