package scanner

import (
	"context"
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

// Scanner handles discovery and monitoring of .agent/skills directories.
type Scanner struct {
	Roots   []string
	Watcher *fsnotify.Watcher
}

// NewScanner initializes a new scanner with the provided root directories.
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
func FindProjectSkillsRoots() []string {
	var roots []string
	cwd, err := os.Getwd()
	if err != nil {
		return roots
	}

	searchCwd := cwd
	for {
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

// Discover finds all SKILL.md files using parallel walking.
func (s *Scanner) Discover(ctx context.Context) ([]string, error) {
	var (
		mu         sync.Mutex
		skillFiles []string
	)
	g, gCtx := errgroup.WithContext(ctx)

	for _, root := range s.Roots {
		if root == "" {
			continue
		}

		r := root
		g.Go(func() error {
			return filepath.WalkDir(r, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil
				}

				select {
				case <-gCtx.Done():
					return gCtx.Err()
				default:
				}

				rel, err := filepath.Rel(r, path)
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
		})
	}

	if err := g.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		slog.Warn("Discovery produced some root errors", "error", err)
	}

	cwd, _ := os.Getwd()
	slices.SortFunc(skillFiles, func(a, b string) int {
		aLocal := strings.HasPrefix(a, cwd)
		bLocal := strings.HasPrefix(b, cwd)
		if aLocal && !bLocal {
			return 1
		}
		if !aLocal && bLocal {
			return -1
		}
		return strings.Compare(a, b)
	})

	return skillFiles, nil
}

// Listen starts a callback loop for incremental updates.
func (s *Scanner) Listen(ctx context.Context, onUpdate func(path string), onDelete func(path string)) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
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
