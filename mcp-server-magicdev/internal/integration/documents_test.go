package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"mcp-server-magicdev/internal/db"
)

func TestNormalizeLineEndings(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping normalizeLineEndings on non-windows")
	}
	input := "Line1\nLine2\r\nLine3"
	expected := "Line1\r\nLine2\r\nLine3"
	result := normalizeLineEndings(input)
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestEscapeGenericsOutsideCode(t *testing.T) {
	input := "List<T> \n```go\nList<T>\n```\n`Map<K,V>`"
	expected := "List&lt;T&gt; \n```go\nList<T>\n```\n`Map<K,V>`"
	result := escapeGenericsOutsideCode(input)
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestMarkdownToXHTML(t *testing.T) {
	input := "Hello\n---\n<br>\nList<T>"
	result, err := MarkdownToXHTML(input)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !strings.Contains(result, "Hello") || !strings.Contains(result, "&lt;T&gt;") {
		t.Errorf("Unexpected output: %q", result)
	}
}

func TestVerifyCrossLinks(t *testing.T) {
	// Should pass
	err := verifyCrossLinks([]byte("JIRA-123"), "JIRA-123", "JIRA-123")
	if err != nil {
		t.Errorf("Expected nil error, got: %v", err)
	}

	// Should pass UNKNOWN
	err = verifyCrossLinks([]byte("missing"), "missing", "UNKNOWN")
	if err != nil {
		t.Errorf("Expected nil error for UNKNOWN, got: %v", err)
	}

	// Should fail after retries
	err = verifyCrossLinks([]byte("JIRA-123"), "missing", "JIRA-123")
	if err == nil {
		t.Error("Expected error for missing crosslink")
	}
}

func TestAppendRoadmapSection(t *testing.T) {
	bp := &db.Blueprint{
		ImplementationStrategy: map[string]string{
			"Step1": "Detail1",
		},
	}
	res := appendRoadmapSection("base markdown", bp)
	if !strings.Contains(res, "base markdown") || !strings.Contains(res, "Technical Implementation Roadmap") || !strings.Contains(res, "Step1") {
		t.Errorf("Unexpected output: %q", res)
	}

	resEmpty := appendRoadmapSection("base markdown", &db.Blueprint{})
	if !strings.Contains(resEmpty, "base markdown") {
		t.Errorf("Expected base markdown, got: %q", resEmpty)
	}
}

func TestGenerateHybridMarkdown(t *testing.T) {
	bp := &db.Blueprint{
		ComplexityScores: map[string]int{"mod1": 3},
		FileStructure: []db.FileEntry{{Path: "main.go"}},
		ADRs: []db.ADR{{Title: "adr1"}},
		DependencyManifest: []db.Dependency{{Name: "dep1"}},
	}
	synth := &db.SynthesisResolution{
		Decisions: []db.ArchitecturalDecision{{Topic: "t1"}},
	}
	res, err := generateHybridMarkdown("JIRA-123", "markdown", bp, synth)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	resStr := string(res)
	if !strings.Contains(resStr, "JIRA-123") || !strings.Contains(resStr, "3") || !strings.Contains(resStr, "markdown") {
		t.Errorf("Missing expected hybrid content: %q", resStr)
	}
}

func TestProcessDocumentGeneration(t *testing.T) {
	// Save current state
	oldJiraDisable := viper.GetBool("jira.disable")
	oldConfDisable := viper.GetBool("confluence.disable")
	oldGitLabDisable := viper.GetBool("gitlab.disable")
	oldGitHubDisable := viper.GetBool("github.disable")
	oldJiraIssue := viper.GetString("jira.issue")
	oldParentID := viper.GetString("confluence.parent_page_id")
	oldGitLabServer := viper.GetString("gitlab.server_url")
	oldGitLabProject := viper.GetString("gitlab.project_path")
	oldGitHubServer := viper.GetString("github.server_url")
	oldGitHubProject := viper.GetString("github.project_path")
	
	defer func() {
		viper.Set("jira.disable", oldJiraDisable)
		viper.Set("confluence.disable", oldConfDisable)
		viper.Set("gitlab.disable", oldGitLabDisable)
		viper.Set("github.disable", oldGitHubDisable)
		viper.Set("jira.issue", oldJiraIssue)
		viper.Set("confluence.parent_page_id", oldParentID)
		viper.Set("gitlab.server_url", oldGitLabServer)
		viper.Set("gitlab.project_path", oldGitLabProject)
		viper.Set("github.server_url", oldGitHubServer)
		viper.Set("github.project_path", oldGitHubProject)
	}()

	// Set up mocks
	viper.Set("jira.disable", true)
	viper.Set("confluence.disable", true)
	viper.Set("gitlab.disable", true)
	viper.Set("github.disable", true)
	viper.Set("jira.issue", "") // let it create one
	viper.Set("confluence.parent_page_id", "")
	viper.Set("gitlab.server_url", "http://example.com")
	viper.Set("gitlab.project_path", "test/project")
	viper.Set("github.server_url", "http://example.com")
	viper.Set("github.project_path", "owner/repo")
	
	dbPath := filepath.Join(os.TempDir(), "test_documents.db")
	viper.Set("server.db_path", dbPath)
	defer os.Remove(dbPath)

	// Ensure store
	store, _ := db.InitStore()
	defer store.Close()
	
	store.SetSecret("gitlab", "dummy-token")
	
	session := &db.SessionState{
		SessionID: "test-sess",
	}
	store.SaveSession(session)
	
	bp := &db.Blueprint{
		ComplexityScores: map[string]int{"mod1": 3},
		FileStructure: []db.FileEntry{{Path: "main.go"}},
	}
	synth := &db.SynthesisResolution{
		Decisions: []db.ArchitecturalDecision{{Topic: "t1"}},
	}
	
	jiraID, _, _, err := ProcessDocumentGeneration(store, "Test Title", "Test Markdown", "main", "test-sess", bp, synth)
	if err != nil {
		t.Errorf("ProcessDocumentGeneration returned error: %v", err)
	}
	if jiraID != "skipped (disabled)" {
		t.Errorf("Expected jiraID to be skipped (disabled) for mocked test, got %s", jiraID)
	}
}
