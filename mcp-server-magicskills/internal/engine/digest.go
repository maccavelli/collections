package engine

import (
	"fmt"
	"mcp-server-magicskills/internal/models"
	"regexp"
	"strings"
)

var sectionRegex = regexp.MustCompile(`(?m)^##\s+(.*)$`)

// GenerateDigest executes the designated operation.
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

// Pre-compiled filler patterns for Densify (eliminates per-call regex compilation).
// Articles (the, a, an) deliberately excluded — they break technical content.
var densifyFillers = func() []*regexp.Regexp {
	patterns := []string{
		`you should`, `please ensure`, `it is important to`, `the user needs to`,
		`make sure to`, `keep in mind that`, `as a result of`, `note that`,
		`basically`, `simply`, `it is worth noting`, `feel free to`,
	}
	compiled := make([]*regexp.Regexp, len(patterns))
	for i, f := range patterns {
		compiled[i] = regexp.MustCompile(`(?i)` + f)
	}
	return compiled
}()

var whitespaceRegex = regexp.MustCompile(`\s+`)

// Densify condenses text by removing fillers while preserving casing behavior via regex case-insensitive matching!
func Densify(text string) string {
	lines := strings.Split(text, "\n")
	var dense []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		for _, re := range densifyFillers {
			trimmed = re.ReplaceAllString(trimmed, "")
		}

		// Cleanup double spaces that may be left over
		trimmed = whitespaceRegex.ReplaceAllString(trimmed, " ")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}

		// Using literal replacements for standard formatting adjustments
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
