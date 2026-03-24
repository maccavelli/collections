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

// Version is overwritten by build flags
var Version = "2.0.0"

const (
	AppTitle = "prepare-commit-msg"
	Timeout  = 120 * time.Second
)

func main() {
	setup := flag.Bool("setup", false, "run interactive setup")
	ver := flag.Bool("version", false, "show version")
	flag.Parse()

	if *ver {
		fmt.Printf("%s version %s\n", AppTitle, Version)
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

	msg = llm.Clean(msg)
	if len(msg) < 5 {
		return fmt.Errorf("AI message too short or empty after cleaning")
	}

	return writeMessage(file, msg, info)
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
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing message: %w", err)
	}

	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	fmt.Fprintln(f, strings.TrimSpace(msg))
	fmt.Fprintln(f, "")
	fmt.Fprintf(f, "# AI-generated (%d files: +%d -%d)\n", len(info.Files), info.Additions, info.Deletions)
	fmt.Fprintf(f, "# %s\n\n", info.Stats)
	_, err = f.Write(existing)
	f.Close()

	if err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, path)
}
