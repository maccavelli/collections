package tags

import (
	"context"
	"testing"
)

type TagStruct struct {
	FirstName string
	LastName  string `json:"old_last"`
}

func TestAnalyzeTags(t *testing.T) {
	result, err := AnalyzeTags(context.Background(), "mcp-server-go-refactor/internal/tags", "TagStruct", "snake", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected tag modification data")
	}
	
	found := false
	for _, m := range result.Modifications {
		if m.Field == "FirstName" {
			found = true
			if m.SuggestedTag != "`json:\"first_line\"`" && m.SuggestedTag != "`json:\"first_name\"`" {
				// FirstName -> splitWords -> ["first", "name"] -> snake -> first_name
				if m.SuggestedTag != "`json:\"first_name\"`" {
					t.Errorf("unexpected suggestion for FirstName: %s", m.SuggestedTag)
				}
			}
		}
	}
	if !found {
		t.Error("FirstName not found in modifications")
	}
}
