package integration

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"mcp-server-magicdev/internal/db"
)

func TestAppendRoadmapSection(t *testing.T) {
	bp := &db.Blueprint{
		ImplementationStrategy: map[string]string{"req1": "pattern1"},
		DependencyManifest: []db.Dependency{
			{Name: "pkg", Version: "1.0", Ecosystem: "npm"},
		},
		ComplexityScores:   map[string]int{"feature1": 5},
		AporiaTraceability: map[string]string{"contradiction": "resolution"},
	}

	result := appendRoadmapSection("Base Markdown", bp)

	if !strings.Contains(result, "Base Markdown") {
		t.Error("Missing base markdown")
	}
	if !strings.Contains(result, "pattern1") {
		t.Error("Missing pattern1")
	}
	if !strings.Contains(result, "pkg") {
		t.Error("Missing dependency")
	}
	if !strings.Contains(result, "5 SP") {
		t.Error("Missing story points")
	}
	if !strings.Contains(result, "resolution") {
		t.Error("Missing aporia resolution")
	}
}

func TestNormalizeLineEndings(t *testing.T) {
	// Not exhaustive testing for windows runtime, but verifies no panic
	res := normalizeLineEndings("line1\nline2\r\nline3")
	if len(res) == 0 {
		t.Error("Failed to normalize")
	}
}

func TestGenerateHybridMarkdown(t *testing.T) {
	bp := &db.Blueprint{
		DependencyManifest: []db.Dependency{
			{Name: "pkg", Version: "1.0"},
		},
		ComplexityScores: map[string]int{"auth": 5, "api": 8},
		FileStructure: []db.FileEntry{
			{Path: "src/index.ts", Type: "file"},
		},
		ADRs: []db.ADR{
			{Title: "Use TypeScript", Status: "Accepted"},
		},
	}
	synthesis := &db.SynthesisResolution{Narrative: "test synthesis"}

	hybridBytes, err := generateHybridMarkdown("JIRA-1", "content", bp, synthesis)
	if err != nil {
		t.Fatalf("Failed to generate: %v", err)
	}

	output := string(hybridBytes)

	if !strings.Contains(output, "JIRA-1") {
		t.Errorf("Expected JIRA-1 in output")
	}
	if !strings.Contains(output, "pkg") {
		t.Error("Missing dependency manifest in output")
	}
	if !strings.Contains(output, "---json") {
		t.Error("Missing JSON frontmatter block in output")
	}
	// Validate expanded frontmatter fields
	if !strings.Contains(output, "\"schema_version\"") {
		t.Error("Missing schema_version in frontmatter")
	}
	if !strings.Contains(output, "\"generated_at\"") {
		t.Error("Missing generated_at in frontmatter")
	}
	if !strings.Contains(output, "\"total_story_points\": 13") {
		t.Error("Missing or incorrect total_story_points in frontmatter")
	}
	if !strings.Contains(output, "\"file_count\": 1") {
		t.Error("Missing or incorrect file_count in frontmatter")
	}
	if !strings.Contains(output, "\"adr_count\": 1") {
		t.Error("Missing or incorrect adr_count in frontmatter")
	}
	if len(hybridBytes) == 0 {
		t.Error("Payload is empty")
	}
}

func TestGenerateHybridMarkdownNilBlueprint(t *testing.T) {
	hybridBytes, err := generateHybridMarkdown("JIRA-2", "minimal content", nil, nil)
	if err != nil {
		t.Fatalf("Failed to generate with nil bp: %v", err)
	}
	output := string(hybridBytes)
	if !strings.Contains(output, "\"schema_version\": 1") {
		t.Error("Expected schema_version even with nil blueprint")
	}
	if !strings.Contains(output, "minimal content") {
		t.Error("Missing markdown body")
	}
}

func TestProcessDocumentGenerationErrors(t *testing.T) {
	// Should fail fast due to empty configuration
	_, err := ProcessDocumentGeneration(nil, "title", "md", "/invalid/path", "session", nil, nil)
	if err == nil {
		t.Error("Expected error from ProcessDocumentGeneration")
	}
}

func TestProcessDocumentGenerationNetworkFails(t *testing.T) {
	// Temporarily set valid-looking URLs that will fail to connect
	viper.Set("confluence.url", "http://127.0.0.1:0")
	viper.Set("confluence.token", "dummy")
	viper.Set("jira.url", "http://127.0.0.1:0")
	viper.Set("jira.token", "dummy")
	
	bp := &db.Blueprint{
		ComplexityScores: map[string]int{"feature1": 5},
	}
	
	// This will fail at git push because the repo path is invalid,
	// but it will successfully traverse the Jira and Confluence setup logic.
	_, err := ProcessDocumentGeneration(nil, "title", "md", "/invalid/path", "session", bp, &db.SynthesisResolution{Narrative: "test"})
	if err == nil {
		t.Error("Expected error from ProcessDocumentGeneration at git push")
	}
	
	viper.Reset()
}


func TestPushToGitLabError(t *testing.T) {
	err := pushToGitLab(nil, "JIRA-123", "main", "title", []byte("data"), nil)
	if err == nil {
		t.Error("Expected error from pushToGitLab")
	}
}
