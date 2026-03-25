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
	w.Add("app.go")
	w.Add("config.yaml")

	info, err = GatherInfo(32000)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify stats
	if len(info.Files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(info.Files), info.Files)
	}
	if !contains(info.Files, "app.go") || !contains(info.Files, "config.yaml") {
		t.Errorf("missing expected files: %v", info.Files)
	}

	// Verify counts (YAML: 1, Scripts: 1)
	if !strings.Contains(info.Stats, "YAML: 1") || !strings.Contains(info.Stats, "Scripts: 1") {
		t.Errorf("expected stats to contain YAML: 1 and Scripts: 1, got %s", info.Stats)
	}

	// Verify diff
	if !strings.Contains(info.Diff, "diff --git a/app.go b/app.go") {
		t.Errorf("diff missing expected file header: %s", info.Diff)
	}

	// Check additions/deletions
	// app.go is new, 3 lines. config.yaml is new, 1 line.
	// Initial commit has README.md.
	// Staged: app.go (3 insertions), config.yaml (1 insertion)
	if info.Additions < 3 { // Depending on line splits, should be 3 or 4
		t.Errorf("expected additions >= 3, got %d", info.Additions)
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
