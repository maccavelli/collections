package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

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
	if err := os.WriteFile(path, []byte(skillContent), 0600); err != nil {
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
		Sections: map[string]string{"full": "Go development golang"},
		TermFreq: tokenize("Go development golang"),
	}
	e.Skills["python-skill"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "python-skill", Description: "Python automation", Tags: []string{"scripting"}},
		Sections: map[string]string{"full": "Python automation scripting"},
		TermFreq: tokenize("Python automation scripting"),
	}
	e.RecalculateIndices()

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
			"full":            "Full content that is very long...",
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

func TestEngine_IngestSingle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "magicskills-test-single")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	skillContent := `---
name: single-skill
version: 1.0.0
---
## Test
This is a test.
`
	path := filepath.Join(tempDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(skillContent), 0600); err != nil {
		t.Fatal(err)
	}

	e := NewEngine()
	if err := e.IngestSingle(path); err != nil {
		t.Fatalf("IngestSingle failed: %v", err)
	}

	s, ok := e.GetSkill("single-skill")
	if !ok {
		t.Fatal("Skill not found after IngestSingle")
	}
	if s.Metadata.Name != "single-skill" {
		t.Errorf("Expected single-skill, got %s", s.Metadata.Name)
	}

	// Test malformed
	badPath := filepath.Join(tempDir, "BAD.md")
	os.WriteFile(badPath, []byte("NOT A VALID SKILL"), 0600)
	if err := e.IngestSingle(badPath); err == nil {
		t.Error("Expected error for malformed skill in IngestSingle")
	}

	// Test missing file
	if err := e.IngestSingle(filepath.Join(tempDir, "missing.md")); err == nil {
		t.Error("Expected error for missing file in IngestSingle")
	}
}

func TestEngine_LocalPriority(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "magicskills-test-priority")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	cwd, _ := os.Getwd()
	globalPath := filepath.Join(tempDir, "global", "SKILL.md")
	localPath := filepath.Join(cwd, "test-local-SKILL.md") // Simulated local path
	os.MkdirAll(filepath.Dir(globalPath), 0755)

	skillContent := `---
name: priority-skill
version: 1.0.0
---
## Global
`
	os.WriteFile(globalPath, []byte(skillContent), 0600)

	localContent := `---
name: priority-skill
version: 2.0.0
---
## Local
`
	os.WriteFile(localPath, []byte(localContent), 0600)
	defer os.Remove(localPath)

	e := NewEngine()
	// Simulate order: Global then Local
	if err := e.Ingest([]string{globalPath, localPath}); err != nil {
		t.Fatal(err)
	}

	s, _ := e.GetSkill("priority-skill")
	if s.Metadata.Version != "2.0.0" {
		t.Errorf("Expected priority-skill v2.0.0 (local), got v%s", s.Metadata.Version)
	}
}

func TestEngine_Remove(t *testing.T) {
	e := NewEngine()
	path := "/fake/path/SKILL.md"
	name := "to-remove"

	e.Skills[name] = &models.Skill{Metadata: models.SkillMetadata{Name: name}}
	e.PathToName[path] = name

	e.Remove(path)

	if _, ok := e.GetSkill(name); ok {
		t.Error("Skill was not removed")
	}
	if _, ok := e.PathToName[path]; ok {
		t.Error("PathToName mapping was not removed")
	}
}

func BenchmarkMatchSkills_500(b *testing.B) {
	e := NewEngine()
	for i := 0; i < 500; i++ {
		name := fmt.Sprintf("skill-%d", i)
		e.Skills[name] = &models.Skill{
			Metadata: models.SkillMetadata{Name: name, Description: "Typical skill description for indexing."},
			TermFreq: tokenize("Detailed workflow steps for a typical skill in the index with some keywords."),
		}
	}
	e.RecalculateIndices()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.MatchSkills("workflow typical keyword")
	}
}
