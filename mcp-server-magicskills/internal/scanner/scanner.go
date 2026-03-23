package scanner

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sync/errgroup"
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

// FindProjectSkillsRoots looks for .agent/skills starting from CWD and walking up.
// It prioritizes these local directories over any global candidates.
func FindProjectSkillsRoots() []string {
	var roots []string
	cwd, err := os.Getwd()
	if err != nil {
		return roots
	}

	searchCwd := cwd
	for {
		// Local Project Candidates
		for _, name := range []string{".agents/skills", ".agent/skills", ".gemini/skills", ".claude/rules", ".claude/skills", ".cursor/rules", ".github/skills", ".github/instructions"} {
			target := filepath.Join(searchCwd, name)
			if info, err := os.Stat(target); err == nil && info.IsDir() {
				if !contains(roots, target) {
					roots = append(roots, target)
				}
			}
		}

		parent := filepath.Dir(searchCwd)
		if parent == searchCwd {
			break
		}
		searchCwd = parent
	}

	return roots
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// Discover finds all SKILL.md files in the configured roots using parallel walking and shallow scanning
func (s *Scanner) Discover() ([]string, error) {
	var (
		mu         sync.Mutex
		skillFiles []string
		g          errgroup.Group
	)

	for _, root := range s.Roots {
		if root == "" {
			continue
		}

		g.Go(func() error {
			// Shallow Walk: MagicSkills usually reside in root/skill-name/SKILL.md
			// We skip very deep directories to enhance performance
			err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil //nolint:nilerr // Skip inaccessible subtrees
				}

				rel, err := filepath.Rel(root, path)
				if err != nil {
					return fs.SkipDir
				}
				if strings.Count(rel, string(os.PathSeparator)) > 2 {
					return fs.SkipDir
				}

				if !d.IsDir() && d.Name() == "SKILL.md" {
					mu.Lock()
					skillFiles = append(skillFiles, path)
					mu.Unlock()
					if err := s.Watcher.Add(filepath.Dir(path)); err != nil {
						slog.Warn("failed to watch skill directory", "path", filepath.Dir(path), "error", err)
					}
				}
				return nil
			})
			return err
		})
	}

	if err := g.Wait(); err != nil {
		slog.Warn("Discovery warning: some roots failed to scan", "error", err)
	}

	// Sort files to ensure Local skill files (within CWD) are processed after Global ones.
	// This allows Ingest to overwrite global skills with local ones correctly.
	cwd, _ := os.Getwd()
	slices.SortFunc(skillFiles, func(a, b string) int {
		aLocal := strings.HasPrefix(a, cwd)
		bLocal := strings.HasPrefix(b, cwd)
		if aLocal && !bLocal {
			return 1 // a is local, should come after global b
		}
		if !aLocal && bLocal {
			return -1 // a is global, should come before local b
		}
		return strings.Compare(a, b)
	})

	return skillFiles, nil
}

// Listen starts a callback when skill changes occur, supporting incremental updates
func (s *Scanner) Listen(onUpdate func(path string), onDelete func(path string)) {
	go func() {
		for {
			select {
			case event, ok := <-s.Watcher.Events:
				if !ok {
					return
				}

				if strings.HasSuffix(event.Name, "SKILL.md") {
					if event.Has(fsnotify.Write | fsnotify.Create) {
						onUpdate(event.Name)
					} else if event.Has(fsnotify.Remove | fsnotify.Rename) {
						onDelete(event.Name)
					}
				}
			case err, ok := <-s.Watcher.Errors:
				if !ok {
					return
				}
				slog.Error("Watcher error", "error", err)
			}
		}
	}()
}
