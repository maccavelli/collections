package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"iter"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
	"mcp-server-magicskills/internal/models"
)

const (
	CurrentSchemaVersion = "2.0"
)

var (
	sectionRegex = regexp.MustCompile(`(?m)^##\s+(.*)$`)

	builderPool = sync.Pool{
		New: func() any {
			b := new(strings.Builder)
			b.Grow(1024)
			return b
		},
	}
)

type Engine struct {
	mu         sync.RWMutex
	Skills     map[string]*models.Skill
	PathToName map[string]string

	DocFreq   map[string]float64
	AvgDocLen float64
	TotalDocs int
}

func NewEngine() *Engine {
	return &Engine{
		Skills:     make(map[string]*models.Skill),
		PathToName: make(map[string]string),
	}
}

// Ingest loads provided skill files into the engine in a context-aware manner.
func (e *Engine) Ingest(ctx context.Context, paths []string) error {
	var (
		mu     sync.Mutex
		parsed = make(map[string]*models.Skill)
	)
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	for _, path := range paths {
		p := path
		g.Go(func() error {
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			skill, err := parseSkillFile(p, data)
			if err != nil {
				return nil
			}

			select {
			case <-gCtx.Done():
				return gCtx.Err()
			default:
			}

			skill.Hash = hashContent(data)
			skill.Digest = GenerateDigest(skill)
			skill.UpdatedAt = time.Now()
			skill.TokenEstimate = len(skill.Digest) / 4

			mu.Lock()
			parsed[p] = skill
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	results := make(map[string]*models.Skill)
	for _, path := range paths {
		if s, ok := parsed[path]; ok {
			results[s.Metadata.Name] = s
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.Skills = results
	e.PathToName = make(map[string]string, len(results))
	for _, s := range results {
		e.PathToName[s.RawPath] = s.Metadata.Name
	}
	e.RecalculateIndices()
	return nil
}

func (e *Engine) RecalculateIndices() {
	df := make(map[string]float64)
	var totalLen float64
	for _, s := range e.Skills {
		totalLen += float64(len(s.TermFreq))
		for word := range s.TermFreq {
			df[word]++
		}
	}
	e.DocFreq = df
	e.TotalDocs = len(e.Skills)
	if e.TotalDocs > 0 {
		e.AvgDocLen = totalLen / float64(e.TotalDocs)
	}
}

func (e *Engine) IngestSingle(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	hash := hashContent(data)
	if existing, ok := e.Skills[filepath.Base(filepath.Dir(path))]; ok && existing.Hash == hash {
		return nil
	}

	skill, err := parseSkillFile(path, data)
	if err != nil {
		return fmt.Errorf("parse failed: %w", err)
	}
	skill.Hash = hashContent(data)
	skill.Digest = GenerateDigest(skill)
	skill.UpdatedAt = time.Now()
	skill.TokenEstimate = len(skill.Digest) / 4
	skill.SchemaVersion = CurrentSchemaVersion

	e.Skills[skill.Metadata.Name] = skill
	e.PathToName[path] = skill.Metadata.Name
	e.RecalculateIndices()
	return nil
}

func (e *Engine) Remove(ctx context.Context, path string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if name, ok := e.PathToName[path]; ok {
		delete(e.Skills, name)
		delete(e.PathToName, path)
		e.RecalculateIndices()
	}
}

func (e *Engine) GetSkill(name string) (*models.Skill, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.Skills[name]
	return s, ok
}

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
	score float64
}

// MatchSkills returns skills matched via BM25, respecting context deadline.
func (e *Engine) MatchSkills(ctx context.Context, intent string) []*models.Skill {
	e.mu.RLock()
	defer e.mu.RUnlock()

	intent = strings.ToLower(intent)
	keywords := strings.Fields(intent)

	N := float64(e.TotalDocs)
	if N == 0 {
		return nil
	}

	idf := make(map[string]float64, len(keywords))
	for _, kw := range keywords {
		if df := e.DocFreq[kw]; df > 0 {
			idf[kw] = math.Log((N-df+0.5)/(df+0.5) + 1.0)
		} else {
			idf[kw] = 0.001
		}
	}

	matches := make([]scoredSkill, 0, len(e.Skills))
	for _, s := range e.Skills {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		score := 0.0
		weights := map[string]float64{
			strings.ToLower(s.Metadata.Name):        5.0,
			strings.ToLower(s.Metadata.Description): 2.0,
			strings.ToLower(s.Sections["full"]):     0.5,
		}

		for _, kw := range keywords {
			tf := 0.0
			for text, weight := range weights {
				if strings.Contains(text, kw) {
					tf += weight
				}
			}
			for _, tag := range s.Metadata.Tags {
				if strings.Contains(strings.ToLower(tag), kw) {
					tf += 3.0
				}
			}

			if count, ok := s.TermFreq[kw]; ok {
				tf += float64(count) * 0.1
			}

			if tf > 0 {
				score += idf[kw] * (tf * (1.5 + 1.0)) / (tf + 1.5*(1.0-0.75+0.75*(float64(len(s.TermFreq))/e.AvgDocLen)))
			}
		}
		if score > 0 {
			matches = append(matches, scoredSkill{s, score})
		}
	}

	slices.SortFunc(matches, func(a, b scoredSkill) int {
		if b.score > a.score {
			return 1
		}
		if b.score < a.score {
			return -1
		}
		return 0
	})

	result := make([]*models.Skill, len(matches))
	for i, m := range matches {
		result[i] = m.skill
	}
	return result
}

func (e *Engine) Summarize(ctx context.Context, name string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.Skills[name]
	if !ok {
		return "", false
	}

	for _, key := range []string{"magic directive", "directive", "summary"} {
		if dir, ok := s.Sections[key]; ok {
			return dir, true
		}
	}

	full := s.Sections["full"]
	if len(full) > 300 {
		return full[:300] + "...", true
	}
	return full, true
}

func parseSkillFile(path string, data []byte) (*models.Skill, error) {
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
		Metadata:      meta,
		Sections:      sections,
		RawPath:       path,
		TermFreq:      tokenize(content),
		SchemaVersion: CurrentSchemaVersion,
	}, nil
}

func tokenize(text string) map[string]int {
	counts := make(map[string]int)
	words := strings.Fields(strings.ToLower(text))
	for _, w := range words {
		counts[w]++
	}
	return counts
}

func hashContent(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateDigest(s *models.Skill) string {
	b := builderPool.Get().(*strings.Builder)
	defer func() {
		b.Reset()
		builderPool.Put(b)
	}()

	fmt.Fprintf(b, "# %s v%s\n", s.Metadata.Name, s.Metadata.Version)
	if s.Metadata.Description != "" {
		fmt.Fprintf(b, "> %s\n\n", s.Metadata.Description)
	}

	mapping := map[string][]string{
		"DIRECTIVE": {"magic directive", "directive", "persona", "objective"},
		"WORKFLOW":  {"workflow", "routine", "steps", "usage"},
		"PATTERNS":  {"best practices", "rules", "patterns", "guidelines"},
		"CHECKLIST": {"checklist", "tasks", "todo"},
	}

	for category, keywords := range mapping {
		var content string
		for _, kw := range keywords {
			if c, ok := s.Sections[kw]; ok {
				content = c
				break
			}
		}

		if content != "" {
			fmt.Fprintf(b, "## %s\n", category)
			b.WriteString(Densify(content) + "\n\n")
		}
	}

	return strings.TrimSpace(b.String())
}

func Densify(text string) string {
	lines := strings.Split(text, "\n")
	var dense []string

	fillers := []string{
		"you should", "please ensure", "it is important to", "the user needs to",
		"make sure to", "keep in mind that", "as a result of", "note that",
		"basically", "simply", "it is worth noting", "feel free to",
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lowered := strings.ToLower(trimmed)
		for _, f := range fillers {
			lowered = strings.ReplaceAll(lowered, f, "")
		}

		lowered = strings.ReplaceAll(lowered, " the ", " ")
		lowered = strings.ReplaceAll(lowered, " an ", " ")
		lowered = strings.ReplaceAll(lowered, " a ", " ")

		trimmed = strings.TrimSpace(lowered)
		if trimmed == "" {
			continue
		}

		trimmed = strings.ReplaceAll(trimmed, "followed by", "->")
		trimmed = strings.ReplaceAll(trimmed, "resulting in", "=>")
		trimmed = strings.ReplaceAll(trimmed, "requires", "!")

		if trimmed != "" {
			trimmed = strings.ToUpper(trimmed[:1]) + trimmed[1:]
		}

		dense = append(dense, trimmed)
	}

	return strings.Join(dense, "\n")
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
