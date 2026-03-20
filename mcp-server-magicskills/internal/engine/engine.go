package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"iter"

	"mcp-server-magicskills/internal/models"
	"gopkg.in/yaml.v3"
)

var (
	// Pre-compile regexes for section parsing and formatting (idiomatic Go)
	sectionRegex = regexp.MustCompile(`(?m)^##\s+(.*)$`)
)

// Engine manages the skill index and performs parsing/search
type Engine struct {
	mu     sync.RWMutex
	Skills map[string]*models.Skill
}

func NewEngine() *Engine {
	return &Engine{
		Skills: make(map[string]*models.Skill),
	}
}

// Ingest Skill files and index them (Optimized for Go 1.26 memory management)
func (e *Engine) Ingest(paths []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Pre-allocate map capacity to avoid rehashing (Idiom)
	newSkills := make(map[string]*models.Skill, len(paths))
	
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		skill, err := parseSkillFile(path, data)
		if err != nil {
			continue
		}
		
		// Prioritize local project skills (First-seen in path order)
		if _, exists := newSkills[skill.Metadata.Name]; !exists {
			newSkills[skill.Metadata.Name] = skill
		}
	}
	e.Skills = newSkills
	return nil
}

// GetSkill returns a skill by name
func (e *Engine) GetSkill(name string) (*models.Skill, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.Skills[name]
	return s, ok
}

// AllSkills returns an iterator over the indexed skills (Go 1.26 Idiom)
func (e *Engine) AllSkills() iter.Seq[*models.Skill] {
	return func(yield func(*models.Skill) bool) {
		e.mu.RLock()
		defer e.mu.RUnlock()
		for _, s := range e.Skills {
			if !yield(s) {
				return
			}
		}
	}
}

type scoredSkill struct {
	skill *models.Skill
	score int
}

// MatchSkills returns skills matched via weighted scoring (Optimized via slices.SortFunc)
func (e *Engine) MatchSkills(intent string) []*models.Skill {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	intent = strings.ToLower(intent)
	keywords := strings.Fields(intent)
	
	matches := make([]scoredSkill, 0, len(e.Skills))
	for _, s := range e.Skills {
		score := 0
		name := strings.ToLower(s.Metadata.Name)
		desc := strings.ToLower(s.Metadata.Description)
		
		for _, kw := range keywords {
			if strings.Contains(name, kw) {
				score += 5
			}
			if strings.Contains(desc, kw) {
				score += 2
			}
			for _, tag := range s.Metadata.Tags {
				if strings.Contains(strings.ToLower(tag), kw) {
					score += 3
				}
			}
		}
		if score > 0 {
			matches = append(matches, scoredSkill{s, score})
		}
	}

	// Use modern context-safe sorting from slices package
	slices.SortFunc(matches, func(a, b scoredSkill) int {
		return b.score - a.score // Descending
	})

	result := make([]*models.Skill, len(matches))
	for i, m := range matches {
		result[i] = m.skill
	}
	return result
}

// Summarize returns a 300-char pruned version for token efficiency
func (e *Engine) Summarize(name string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.Skills[name]
	if !ok {
		return "", false
	}

	// Priority mapping for directive sections
	for _, key := range []string{"magic directive", "directive", "summary"} {
		if dir, ok := s.Sections[key]; ok {
			return dir, true
		}
	}

	// Fallback with fixed allocation
	full := s.Sections["full"]
	if len(full) > 300 {
		return full[:300] + "...", true
	}
	return full, true
}

func parseSkillFile(path string, data []byte) (*models.Skill, error) {
	// Split with single byte allocation check
	parts := strings.SplitN(string(data), "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid skill format: missing YAML frontmatter")
	}

	var meta models.SkillMetadata
	if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if meta.Name == "" {
		meta.Name = filepath.Base(filepath.Dir(path))
	}

	content := strings.TrimSpace(parts[2])
	sections := parseSections(content)
	sections["full"] = content

	return &models.Skill{
		Metadata: meta,
		Sections: sections,
		RawPath:  path,
	}, nil
}

func parseSections(content string) map[string]string {
	sections := make(map[string]string)
	indices := sectionRegex.FindAllStringSubmatchIndex(content, -1)

	if len(indices) == 0 {
		return sections
	}

	for i, idx := range indices {
		title := strings.TrimSpace(content[idx[2]:idx[3]])
		start := idx[1]
		end := len(content)
		if i+1 < len(indices) {
			end = indices[i+1][0]
		}
		sections[strings.ToLower(title)] = strings.TrimSpace(content[start:end])
	}

	return sections
}
