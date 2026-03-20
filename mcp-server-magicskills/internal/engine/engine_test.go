package engine

import (
	"os"
	"path/filepath"
	"testing"
	"slices"

	"mcp-server-magicskills/internal/models"
)

func TestEngine_Ingest(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "magicskills-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	skillContent := `---
name: test-skill
description: A test skill
tags: ["go", "test"]
version: 1.0.0
---
## Workflow
1. Test step
## Magic Directive
Always test your code.
`
	path := filepath.Join(tempDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewEngine()
	if err := e.Ingest([]string{path}); err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	s, ok := e.GetSkill("test-skill")
	if !ok {
		t.Fatal("Skill not found after ingestion")
	}

	if s.Metadata.Description != "A test skill" {
		t.Errorf("Expected description 'A test skill', got '%s'", s.Metadata.Description)
	}

	if s.Sections["workflow"] == "" {
		t.Error("Workflow section missing after parsing")
	}
}

func TestEngine_MatchSkills(t *testing.T) {
	e := NewEngine()
	e.Skills["go-skill"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "go-skill", Description: "Go development", Tags: []string{"golang"}},
	}
	e.Skills["python-skill"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "python-skill", Description: "Python automation", Tags: []string{"scripting"}},
	}

	t.Run("Match by name", func(t *testing.T) {
		matches := e.MatchSkills("go")
		if len(matches) == 0 || matches[0].Metadata.Name != "go-skill" {
			t.Errorf("Expected go-skill to be top match for 'go'")
		}
	})

	t.Run("Match by description", func(t *testing.T) {
		matches := e.MatchSkills("automation")
		if len(matches) == 0 || matches[0].Metadata.Name != "python-skill" {
			t.Errorf("Expected python-skill to be top match for 'automation'")
		}
	})

	t.Run("Weighted relevance", func(t *testing.T) {
		// 'golang' is a tag in go-skill. 'automation' is in python-skill description.
		// Tags should score higher (+3) than description (+2).
		matches := e.MatchSkills("golang automation")
		if len(matches) < 2 {
			t.Fatal("Expected two matches")
		}
		if matches[0].Metadata.Name != "go-skill" {
			t.Errorf("Expected go-skill to be higher relevance due to tag match")
		}
	})
}

func TestEngine_Summarize(t *testing.T) {
	e := NewEngine()
	e.Skills["test"] = &models.Skill{
		Sections: map[string]string{
			"magic directive": "The magic instruction.",
			"full": "Full content that is very long...",
		},
	}

	t.Run("Specific section priority", func(t *testing.T) {
		summary, ok := e.Summarize("test")
		if !ok || summary != "The magic instruction." {
			t.Errorf("Expected 'The magic instruction.', got '%s'", summary)
		}
	})

	t.Run("Fallback to snippet", func(t *testing.T) {
		e.Skills["fallback"] = &models.Skill{
			Sections: map[string]string{
				"full": "Short content",
			},
		}
		summary, _ := e.Summarize("fallback")
		if summary != "Short content" {
			t.Errorf("Expected 'Short content', got '%s'", summary)
		}
	})
}

func TestEngine_AllSkillsIterator(t *testing.T) {
	e := NewEngine()
	e.Skills["a"] = &models.Skill{Metadata: models.SkillMetadata{Name: "a"}}
	e.Skills["b"] = &models.Skill{Metadata: models.SkillMetadata{Name: "b"}}

	var names []string
	for s := range e.AllSkills() {
		names = append(names, s.Metadata.Name)
	}

	if len(names) != 2 {
		t.Fatalf("Expected 2 skills from iterator, got %d", len(names))
	}
	
	slices.Sort(names)
	if names[0] != "a" || names[1] != "b" {
		t.Errorf("Iterator missed skill names")
	}
}
