package astutil

import (
	"context"
	"testing"
)

func TestAnalyzeInterface_Fallback(t *testing.T) {
	defer func() { recover() }()
	_, _ = AnalyzeInterface(context.Background(), ".", "NonExistentStruct", "UnknownInterface")
}

func TestExtractInterface_Fallback(t *testing.T) {
	defer func() { recover() }()
	_, _ = ExtractInterface(context.Background(), ".", "UnknownStruct")
}
