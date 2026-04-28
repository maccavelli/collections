package server

import (
	"testing"

	"mcp-server-recall/internal/harvest"
)

func TestDetectDomainTags(t *testing.T) {
	tests := []struct {
		name     string
		doc      string
		expected []string
	}{
		{
			name:     "Auth Domain",
			doc:      "This function provides security and auth validation for the APIs.",
			expected: []string{"domain:auth", "domain:api"},
		},
		{
			name:     "Database Domain",
			doc:      "Performs a sql query on the abstract database.",
			expected: []string{"domain:database"}, // "sql" creates database, "database" creates database (set dedup logic naturally handles mapping, but wait, function just appends)
		},
		{
			name:     "Empty Domain",
			doc:      "Just a generic helper function passing values.",
			expected: nil,
		},
		{
			name:     "Case Insensitive",
			doc:      "Orchestrator observability metrics",
			expected: []string{"domain:observability", "domain:metrics", "domain:orchestrator"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := detectDomainTags(tt.doc)
			// verify tags
			for _, exp := range tt.expected {
				found := false
				for _, got := range tags {
					if got == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected tag %s but got %v", exp, tags)
				}
			}
		})
	}
}

func TestBuildSymbolEntry(t *testing.T) {
	sym := harvest.HarvestedSymbol{
		Name:         "TestFunc",
		PkgPath:      "pkg/test",
		Doc:          "Handles auth routing",
		SymbolType:   "func",
		Receiver:     "Server",
		Interfaces:   []string{"Handler"},
		Dependencies: []string{"github.com/gin-gonic/gin"},
	}

	entry, err := buildSymbolEntry(sym, "standards")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Key != "pkg:pkg/test:TestFunc" {
		t.Errorf("expected key pkg:pkg/test:TestFunc, got %s", entry.Key)
	}
	if entry.Category != "HarvestedCode" {
		t.Errorf("expected HarvestedCode category, got %s", entry.Category)
	}

	expectedTags := []string{"harvested", "source:standards", "module:pkg", "type:func", "implements:Handler", "receiver:Server", "depends_on:github.com/gin-gonic/gin", "domain:auth"}
	for _, exp := range expectedTags {
		found := false
		for _, got := range entry.Tags {
			if got == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tag %s but got %v", exp, entry.Tags)
		}
	}
}
