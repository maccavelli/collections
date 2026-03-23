package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestScanner_FindProjectSkillsRoots(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "magicskills-scanner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create structure: tempDir/project/.agent/skills
	projectDir := filepath.Join(tempDir, "project")
	skillsDir := filepath.Join(projectDir, ".agent", "skills")
	if err := os.MkdirAll(skillsDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Change working directory to test finding it
	originalCwd, _ := os.Getwd()
	defer os.Chdir(originalCwd)

	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}

	roots := FindProjectSkillsRoots()
	if len(roots) == 0 {
		t.Fatal("Expected to find project skills roots")
	}

	found := false
	for _, r := range roots {
		if filepath.Base(filepath.Dir(r)) == ".agent" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find .agent/skills in roots")
	}
}

func TestScanner_Discover(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "magicskills-discover-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	skillPath := filepath.Join(tempDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("---\nname: test\n---\n"), 0600); err != nil {
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

func TestScanner_Contains(t *testing.T) {
	slice := []string{"foo", "bar", "baz"}
	if !contains(slice, "bar") {
		t.Error("contains should return true for 'bar'")
	}
	if contains(slice, "unknown") {
		t.Error("contains should return false for 'unknown'")
	}
}

func TestScanner_Listen(t *testing.T) {
	s, err := NewScanner([]string{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Watcher.Close()

	updatedChan := make(chan string, 1)
	deletedChan := make(chan string, 1)

	s.Listen(func(path string) {
		updatedChan <- path
	}, func(path string) {
		deletedChan <- path
	})

	// Simulate event
	s.Watcher.Events <- fsnotify.Event{Name: "/path/to/SKILL.md", Op: fsnotify.Write}
	val := <-updatedChan
	if val != "/path/to/SKILL.md" {
		t.Errorf("Expected listen write event, got: %s", val)
	}

	s.Watcher.Events <- fsnotify.Event{Name: "/path/to/SKILL.md", Op: fsnotify.Remove}
	val = <-deletedChan
	if val != "/path/to/SKILL.md" {
		t.Errorf("Expected listen remove event, got: %s", val)
	}
}
