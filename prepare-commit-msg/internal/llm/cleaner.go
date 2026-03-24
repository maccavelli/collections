package llm

import (
	"regexp"
	"strings"
)

var (
	markdownCodeFence = regexp.MustCompile("(?s)```[a-zA-Z]*\n?(.*?)\n?```")
	fillerPrefixes    = regexp.MustCompile(`(?i)^((based on|here is|generated|suggested|commit message|the following|content).*?[:!]|---)$`)
)

// Clean strips markdown code fences, conversational fillers, and leading/trailing whitespace.
func Clean(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}

	// 1. Strip markdown fences if present
	if matches := markdownCodeFence.FindStringSubmatch(out); len(matches) > 1 {
		out = strings.TrimSpace(matches[1])
	}

	// 2. Strip conversational fillers from all lines
	lines := strings.Split(out, "\n")
	var result []string
	
	for _, line := range lines {
		tl := strings.TrimSpace(line)
		
		// Skip known filler lines anywhere (headers, separators, trailing help text)
		if fillerPrefixes.MatchString(tl) {
			continue
		}
		
		result = append(result, line)
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}
