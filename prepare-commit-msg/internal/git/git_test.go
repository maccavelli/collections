package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseShortStat_Insertions(t *testing.T) {
	tests := []struct {
		name string
		line string
		term string
		want int
	}{
		{"insertions", " 3 files changed, 42 insertions(+), 10 deletions(-)", "insertion", 42},
		{"deletions", " 3 files changed, 42 insertions(+), 10 deletions(-)", "deletion", 10},
		{"no match", " 3 files changed", "insertion", 0},
		{"empty", "", "insertion", 0},
		{"insertions only", " 1 file changed, 5 insertions(+)", "insertion", 5},
		{"deletions only", " 1 file changed, 3 deletions(-)", "deletion", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseShortStat(tt.line, tt.term)
			if got != tt.want {
				t.Errorf("parseShortStat(%q, %q) = %d, want %d", tt.line, tt.term, got, tt.want)
			}
		})
	}
}

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
