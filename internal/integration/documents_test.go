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
	}
	aporias := []string{"aporia1"}

	hybrid, err := generateHybridMarkdown("JIRA-1", "content", bp, aporias)
	if err != nil {
		t.Fatalf("Failed to generate: %v", err)
	}

	if hybrid.Metadata.JiraID != "JIRA-1" {
		t.Errorf("Expected JIRA-1, got %s", hybrid.Metadata.JiraID)
	}
	if len(hybrid.Metadata.DependencyManifest) != 1 {
		t.Error("Missing dependency manifest in metadata")
	}
	if len(hybrid.Metadata.AporiaResolutions) != 1 {
		t.Error("Missing aporia resolutions in metadata")
	}
	if hybrid.Payload == "" {
		t.Error("Payload is empty")
	}
}

func TestProcessDocumentGenerationErrors(t *testing.T) {
	// Should fail fast due to empty configuration
	err := ProcessDocumentGeneration("title", "md", "/invalid/path", "session", nil, nil)
	if err == nil {
		t.Error("Expected error from ProcessDocumentGeneration")
	}
}

func TestProcessDocumentGenerationNetworkFails(t *testing.T) {
	// Temporarily set valid-looking URLs that will fail to connect
	viper.Set("atlassian_url", "http://127.0.0.1:0")
	viper.Set("atlassian_token", "dummy")
	
	bp := &db.Blueprint{
		ComplexityScores: map[string]int{"feature1": 5},
	}
	
	// This will fail at git push because the repo path is invalid,
	// but it will successfully traverse the Jira and Confluence setup logic.
	err := ProcessDocumentGeneration("title", "md", "/invalid/path", "session", bp, []string{"aporia"})
	if err == nil {
		t.Error("Expected error from ProcessDocumentGeneration at git push")
	}
	
	viper.Reset()
}


func TestPushToGitLabError(t *testing.T) {
	err := pushToGitLab("/does/not/exist", "title", []byte("data"))
	if err == nil {
		t.Error("Expected error from pushToGitLab")
	}
}
