package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"prepare-commit-msg/internal/config"
	"prepare-commit-msg/internal/git"
	"prepare-commit-msg/internal/llm"
	"prepare-commit-msg/internal/ui"
)

var version = "2.0.0"

const (
	AppTitle = "prepare-commit-msg"
	Timeout  = 120 * time.Second
)

func main() {
	setup := flag.Bool("setup", false, "run interactive setup")
	ver := flag.Bool("version", false, "show version")
	flag.Parse()

	if *ver {
		fmt.Printf("%s version %s\n", AppTitle, version)
		return
	}

	conf, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if *setup {
		if err := ui.RunSetup(conf); err != nil {
			fmt.Fprintf(os.Stderr, "Setup failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Git hook logic
	args := flag.Args()
	if len(args) < 1 {
		fmt.Printf("Usage: %s <commit_msg_file> [commit_source]\n", AppTitle)
		return
	}

	commitMsgFile := args[0]
	commitSource := ""
	if len(args) > 1 {
		commitSource = args[1]
	}

	// Skip for non-default sources (merge, squash, etc.)
	if commitSource != "" {
		return
	}

	if !git.IsCommitMsgEmpty(commitMsgFile) {
		return
	}

	if err := runAnalyzer(commitMsgFile, conf); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runAnalyzer(file string, conf *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	info, err := git.GatherInfo()
	if err != nil {
		return err
	}
	if info == nil || len(info.Files) == 0 {
		return nil
	}

	pc, err := conf.GetActive()
	if err != nil {
		return err
	}

	var provider llm.Provider
	switch conf.ActiveProvider {
	case "openai":
		provider = llm.NewOpenAI(pc.APIKey, pc.Model)
	case "gemini":
		p, err := llm.NewGemini(ctx, pc.APIKey, pc.Model)
		if err != nil {
			return err
		}
		provider = p
	default:
		return fmt.Errorf("unsupported provider: %s", conf.ActiveProvider)
	}

	prompt := buildPrompt(info)
	fmt.Fprintf(os.Stderr, "Generating commit message via %s (%s)...\n", provider.Name(), pc.Model)

	msg, err := provider.Generate(ctx, prompt)
	if err != nil {
		return err
	}

	msg = cleanLLMOutput(msg)
	if len(msg) < 5 {
		return fmt.Errorf("AI message too short or empty after cleaning")
	}

	return writeMessage(file, msg, info)
}

func cleanLLMOutput(out string) string {
	// Remove code fences
	out = strings.TrimSpace(out)
	if strings.HasPrefix(out, "```") {
		lines := strings.Split(out, "\n")
		if len(lines) > 2 {
			out = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	// Remove LLM noise
	lines := strings.Split(out, "\n")
	var cleaned []string
	for _, l := range lines {
		tl := strings.TrimSpace(l)
		if tl == "" || tl == "---" || strings.HasPrefix(tl, "Based on") || strings.HasPrefix(tl, "Generate") || strings.HasPrefix(tl, "Here is") {
			continue
		}
		cleaned = append(cleaned, l)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func buildPrompt(info *git.Info) string {
	return fmt.Sprintf(`Generate a conventional commit message for these changes.

IMPORTANT: Return ONLY the commit message. Do not include markdown code fences, conversational filler, or introductory text.

FILES CHANGED:
%s

STATS: %s (+%d, -%d)

DIFF:
%s

---
Format: type(scope): brief description (max 72 chars)
Body: Concise bullet points of technical changes.`, strings.Join(info.Files, "\n"), info.Stats, info.Additions, info.Deletions, info.Diff)
}

func writeMessage(path, msg string, info *git.Info) error {
	existing, _ := os.ReadFile(path)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintln(f, strings.TrimSpace(msg))
	fmt.Fprintln(f, "")
	fmt.Fprintf(f, "# AI-generated (%d files: +%d -%d)\n", len(info.Files), info.Additions, info.Deletions)
	fmt.Fprintf(f, "# %s\n\n", info.Stats)
	_, err = f.Write(existing)
	return err
}
