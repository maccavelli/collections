package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		input string
		want  string
	}{
		{"~/foo", filepath.Join(home, "foo")},
		{"~", filepath.Join(home, "")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}
	for _, tc := range tests {
		got := ExpandHome(tc.input)
		if got != tc.want {
			t.Errorf("ExpandHome(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestIsPathWithinAllowed(t *testing.T) {
	tests := []struct {
		path    string
		allowed []string
		want    bool
	}{
		{"/home/user/docs/file.txt", []string{"/home/user"}, true},
		{"/home/user", []string{"/home/user"}, true},
		{"/etc/passwd", []string{"/home/user"}, false},
		{"", []string{"/home"}, false},
		{"/home/user", []string{}, false},
		{"/home/user\x00evil", []string{"/home"}, false},
	}
	for _, tc := range tests {
		got := IsPathWithinAllowed(tc.path, tc.allowed)
		if got != tc.want {
			t.Errorf("IsPathWithinAllowed(%q, %v) = %v, want %v", tc.path, tc.allowed, got, tc.want)
		}
	}
}

func TestValidatePath(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0755)
	existingFile := filepath.Join(subDir, "test.txt")
	os.WriteFile(existingFile, []byte("data"), 0644)

	m := NewManager([]string{dir})

	// Existing file should validate.
	got, err := m.ValidatePath(existingFile)
	if err != nil {
		t.Fatalf("ValidatePath(%q): %v", existingFile, err)
	}
	if got == "" {
		t.Error("expected non-empty path")
	}

	// New file in allowed dir should validate.
	newFile := filepath.Join(subDir, "new.txt")
	got, err = m.ValidatePath(newFile)
	if err != nil {
		t.Fatalf("ValidatePath(%q): %v", newFile, err)
	}
	if got != newFile {
		t.Errorf("got %q, want %q", got, newFile)
	}

	// Path outside allowed dirs should fail.
	_, err = m.ValidatePath("/etc/passwd")
	if err == nil {
		t.Error("expected error for path outside allowed directories")
	}
}
