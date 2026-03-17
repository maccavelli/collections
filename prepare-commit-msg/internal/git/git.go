package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Info struct {
	Files     []string
	Stats     string
	Additions int
	Deletions int
	Diff      string
}

func GatherInfo() (*Info, error) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git binary not found")
	}

	info := &Info{}

	// 1. Get file names and statuses
	cmd := exec.Command(gitBin, "diff", "--cached", "--name-status")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil // No staged changes or not a repo
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return info, nil
	}

	var counts struct {
		yaml, json, tf, ci, script, other int
	}

	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		f := parts[1]
		info.Files = append(info.Files, f)

		lower := strings.ToLower(f)
		switch {
		case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
			counts.yaml++
		case strings.HasSuffix(lower, ".json"):
			counts.json++
		case strings.HasSuffix(lower, ".tf"), strings.HasSuffix(lower, ".tfvars"):
			counts.tf++
		case strings.Contains(lower, "gitlab-ci"), strings.Contains(lower, "jenkinsfile"):
			counts.ci++
		case strings.HasSuffix(lower, ".sh"), strings.HasSuffix(lower, ".py"), strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".go"):
			counts.script++
		default:
			counts.other++
		}
	}

	var stats []string
	if counts.yaml > 0 { stats = append(stats, fmt.Sprintf("YAML: %d", counts.yaml)) }
	if counts.json > 0 { stats = append(stats, fmt.Sprintf("JSON: %d", counts.json)) }
	if counts.tf > 0 { stats = append(stats, fmt.Sprintf("Terraform: %d", counts.tf)) }
	if counts.ci > 0 { stats = append(stats, fmt.Sprintf("CI/CD: %d", counts.ci)) }
	if counts.script > 0 { stats = append(stats, fmt.Sprintf("Scripts: %d", counts.script)) }
	if counts.other > 0 { stats = append(stats, fmt.Sprintf("Other: %d", counts.other)) }
	info.Stats = strings.Join(stats, ", ")

	// 2. Get additions/deletions
	cmd = exec.Command(gitBin, "diff", "--cached", "--shortstat")
	out, _ = cmd.Output()
	statLine := string(out)
	info.Additions = parseShortStat(statLine, "insertion")
	info.Deletions = parseShortStat(statLine, "deletion")

	// 3. Get actual diff
	cmd = exec.Command(gitBin, "diff", "--cached", "--unified=3")
	out, err = cmd.Output()
	if err == nil {
		info.Diff = string(out)
	}

	return info, nil
}

func parseShortStat(line, term string) int {
	parts := strings.Split(line, ",")
	for _, p := range parts {
		if strings.Contains(p, term) {
			var val int
			fmt.Sscanf(strings.TrimSpace(p), "%d", &val)
			return val
		}
	}
	return 0
}

func IsCommitMsgEmpty(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	// Simple check: if file has non-comment lines, it's not empty
	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return false
		}
	}
	return true
}
