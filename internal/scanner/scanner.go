package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// Scanner handles discovery and monitoring of .agent/skills directories
type Scanner struct {
	Roots   []string
	Watcher *fsnotify.Watcher
}

func NewScanner(roots []string) (*Scanner, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}
	return &Scanner{
		Roots:   roots,
		Watcher: watcher,
	}, nil
}

// FindProjectSkillsRoot looks for .agent/skills starting from CWD and walking up to .git
func FindProjectSkillsRoot() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}

	for {
		// Check for .agent/skills
		target := filepath.Join(cwd, ".agent/skills")
		if info, err := os.Stat(target); err == nil && info.IsDir() {
			return target, true
		}

		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return "", false
}

// Discover finds all SKILL.md files in the configured roots using WalkDir (Go 1.26 preference)
func (s *Scanner) Discover() ([]string, error) {
	var skillFiles []string
	for _, root := range s.Roots {
		if root == "" {
			continue
		}
		
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // Skip errors
			}
			if !d.IsDir() && d.Name() == "SKILL.md" {
				skillFiles = append(skillFiles, path)
				// Watch the parent directory for changes
				_ = s.Watcher.Add(filepath.Dir(path))
			}
			return nil
		})
		
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to walk %s: %v\n", root, err)
		}
	}
	return skillFiles, nil
}

// Listen starts a callback when skill changes occur
func (s *Scanner) Listen(onUpdate func()) {
	go func() {
		for {
			select {
			case event, ok := <-s.Watcher.Events:
				if !ok {
					return
				}
				// Use modern bitwise checks
				if event.Has(fsnotify.Write | fsnotify.Create | fsnotify.Remove) {
					if strings.HasSuffix(event.Name, "SKILL.md") {
						onUpdate()
					}
				}
			case err, ok := <-s.Watcher.Errors:
				if !ok {
					return
				}
				fmt.Fprintf(os.Stderr, "Watcher error: %v\n", err)
			}
		}
	}()
}
