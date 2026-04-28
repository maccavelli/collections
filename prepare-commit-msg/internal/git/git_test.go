package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
)


func TestIsCommitMsgEmpty_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "COMMIT_MSG")

	// Empty file
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if !IsCommitMsgEmpty(path) {
		t.Error("expected empty file to return true")
	}
}

func TestIsCommitMsgEmpty_CommentsOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "COMMIT_MSG")

	content := "# Please enter the commit message\n# Lines starting with '#' are ignored\n\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if !IsCommitMsgEmpty(path) {
		t.Error("expected comments-only file to return true")
	}
}

func TestIsCommitMsgEmpty_WithMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "COMMIT_MSG")

	content := "feat: add feature\n# comment\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if IsCommitMsgEmpty(path) {
		t.Error("expected file with message to return false")
	}
}

func TestIsCommitMsgEmpty_NonExistent(t *testing.T) {
	if !IsCommitMsgEmpty("/nonexistent/path/COMMIT_MSG") {
		t.Error("expected nonexistent file to return true")
	}
}

func TestGatherInfo(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Not a git repo
	info, err := GatherInfo(32000)
	if err != nil {
		t.Errorf("expected no error for non-git repo, got %v", err)
	}
	if info == nil || len(info.Files) != 0 {
		t.Errorf("expected empty info, got %+v", info)
	}

	// Initialize repo
	r, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	// First commit
	w, _ := r.Worktree()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644)
	w.Add("README.md")
	_, _ = w.Commit("Initial commit", &git.CommitOptions{})

	// Stage changes for testing
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("timeout: 30s\n"), 0644)
	os.WriteFile(filepath.Join(dir, "test.json"), []byte("{}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.tf"), []byte("resource {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "Jenkinsfile"), []byte("pipeline {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "other.txt"), []byte("text\n"), 0644)

	w.Add("app.go")
	w.Add("config.yaml")
	w.Add("test.json")
	w.Add("main.tf")
	w.Add("Jenkinsfile")
	w.Add("other.txt")

	info, err = GatherInfo(32000)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify counts (YAML: 1, Scripts: 1, JSON: 1, Terraform: 1, CI/CD: 1, Other: 1)
	if !strings.Contains(info.Stats, "JSON: 1") || !strings.Contains(info.Stats, "Terraform: 1") ||
		!strings.Contains(info.Stats, "CI/CD: 1") || !strings.Contains(info.Stats, "Other: 1") {
		t.Errorf("stats missing some categories: %s", info.Stats)
	}

	// Test truncation logic with a very small maxDiffBytes limit
	infoTruncated, err := GatherInfo(5)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(infoTruncated.Diff, "[diff truncated]") {
		t.Error("expected truncated diff string")
	}
}

func TestGatherInfo_MissingGit(t *testing.T) {
	oldFn := lookPath
	defer func() { lookPath = oldFn }()

	lookPath = func(file string) (string, error) {
		return "", os.ErrNotExist
	}

	_, err := GatherInfo(32000)
	if err == nil {
		t.Error("expected error when git is missing, got nil")
	}
}

func contains(arr []string, s string) bool {
	for _, item := range arr {
		if item == s {
			return true
		}
	}
	return false
}
