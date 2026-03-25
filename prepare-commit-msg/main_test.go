package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prepare-commit-msg/internal/git"
)

func TestCleanLLMOutput_PlainText(t *testing.T) {
	input := "feat(config): add new setting\n\n- Added timeout parameter"
	got := cleanLLMOutput(input)
	if !strings.Contains(got, "feat(config)") {
		t.Errorf("expected commit message preserved, got: %q", got)
	}
}

func TestCleanLLMOutput_CodeFences(t *testing.T) {
	input := "```\nfeat(api): update endpoint\n\n- Changed route\n```"
	got := cleanLLMOutput(input)
	if strings.Contains(got, "```") {
		t.Errorf("expected code fences removed, got: %q", got)
	}
	if !strings.Contains(got, "feat(api)") {
		t.Errorf("expected content preserved, got: %q", got)
	}
}

func TestCleanLLMOutput_CodeFencesWithLanguage(t *testing.T) {
	input := "```text\nfix(build): correct path\n```"
	got := cleanLLMOutput(input)
	if strings.Contains(got, "```") {
		t.Errorf("expected code fences removed, got: %q", got)
	}
	if !strings.Contains(got, "fix(build)") {
		t.Errorf("expected content preserved, got: %q", got)
	}
}

func TestCleanLLMOutput_TrailingTextAfterFence(t *testing.T) {
	input := "```\nfeat(core): new feature\n```\nHere is your commit message!"
	got := cleanLLMOutput(input)
	if strings.Contains(got, "Here is") {
		t.Errorf("expected trailing text after fence removed, got: %q", got)
	}
	if !strings.Contains(got, "feat(core)") {
		t.Errorf("expected content preserved, got: %q", got)
	}
}

func TestCleanLLMOutput_FillerLines(t *testing.T) {
	input := "Based on the changes:\nfeat(ui): update button\n---\nHere is the message"
	got := cleanLLMOutput(input)
	if strings.Contains(got, "Based on") {
		t.Errorf("expected filler removed, got: %q", got)
	}
	if strings.Contains(got, "---") {
		t.Errorf("expected separator removed, got: %q", got)
	}
	if !strings.Contains(got, "feat(ui)") {
		t.Errorf("expected content preserved, got: %q", got)
	}
}

func TestCleanLLMOutput_EmptyInput(t *testing.T) {
	got := cleanLLMOutput("")
	if got != "" {
		t.Errorf("expected empty output for empty input, got: %q", got)
	}
}

func TestBuildPrompt(t *testing.T) {
	info := &git.Info{
		Files:     []string{"app.go", "config.yaml"},
		Stats:     "Scripts: 1, YAML: 1",
		Additions: 10,
		Deletions: 5,
		Diff:      "diff content",
	}

	prompt := buildPrompt(info)
	if !strings.Contains(prompt, "app.go") {
		t.Errorf("expected file list in prompt, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Scripts: 1") {
		t.Errorf("expected stats in prompt, got: %s", prompt)
	}
	if !strings.Contains(prompt, "diff content") {
		t.Errorf("expected diff in prompt, got: %s", prompt)
	}
}

func TestWriteMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "COMMIT_EDITMSG")
	msg := "feat: test message"
	info := &git.Info{Stats: "test stats"}

	err := writeMessage(path, msg, info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), msg) {
		t.Errorf("expected message in file, got: %s", string(content))
	}
	if !strings.Contains(string(content), "test stats") {
		t.Errorf("expected stats in file, got: %s", string(content))
	}
}
