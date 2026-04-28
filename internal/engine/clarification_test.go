package engine

import (
	"context"
	"testing"

	"github.com/tidwall/buntdb"
)

func TestClarifyRequirements(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)

	tests := []struct {
		name             string
		requirements     string
		expectForks      int
		expectForksNames []string
	}{
		{
			name:             "Database trigger",
			requirements:     "I need a scalable database for my app.",
			expectForks:      1,
			expectForksNames: []string{"Database"},
		},
		{
			name:             "Queue and Auth trigger",
			requirements:     "The system needs a message queue and auth logic.",
			expectForks:      2,
			expectForksNames: []string{"Queue", "Auth"},
		},
		{
			name:         "Already defined",
			requirements: "Use a Postgres database with strict consistency.",
			expectForks:  0, // 'Strict' is found in text, so it's defined
		},
		{
			name:         "No match",
			requirements: "Just a hello world script.",
			expectForks:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := e.ClarifyRequirements(context.Background(), tt.requirements, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(resp.Data.Forks) != tt.expectForks {
				t.Errorf("expected %d forks, got %d", tt.expectForks, len(resp.Data.Forks))
			}

			for _, expectedName := range tt.expectForksNames {
				found := false
				for _, f := range resp.Data.Forks {
					if f.Component == expectedName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected fork for %s not found", expectedName)
				}
			}
		})
	}
}
