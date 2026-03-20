package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanner_FindProjectSkillsRoot(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "magicskills-scanner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create structure: tempDir/project/.agent/skills
	projectDir := filepath.Join(tempDir, "project")
	skillsDir := filepath.Join(projectDir, ".agent", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Change working directory to test finding it
	originalCwd, _ := os.Getwd()
	defer os.Chdir(originalCwd)

	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}

	root, ok := FindProjectSkillsRoot()
	if !ok {
		t.Fatal("Expected to find project skills root")
	}

	if !filepath.IsAbs(root) {
		t.Errorf("Expected absolute path, got %s", root)
	}

	if !os.IsPathSeparator(root[0]) && len(root) > 1 && root[1] != ':' {
		// Just a sanity check for Unix style or Windows
	}
}

func TestScanner_Discover(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "magicskills-discover-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	skillPath := filepath.Join(tempDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("---\nname: test\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := NewScanner([]string{tempDir})
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}

	files, err := s.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 skill file, got %d", len(files))
	}

	if files[0] != skillPath {
		t.Errorf("Expected path %s, got %s", skillPath, files[0])
	}
}
