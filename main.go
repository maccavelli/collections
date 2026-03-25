package main

import (
	"bufio"
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

// Version is overwritten by build flags during the compilation process to represent
// the current release version of the application.
var Version = "2.0.0"

const (
	// AppTitle is the name of the application used in help text and version output.
	AppTitle = "prepare-commit-msg"
)

// main is the entry point of the application. It handles flag parsing, configuration loading,
// and determines whether to run the setup wizard or execute the git hook logic.
func main() {
	setup := flag.Bool("setup", false, "run interactive setup")
	ver := flag.Bool("version", false, "show version")
	flag.Parse()

	if *ver {
		fmt.Printf("%s version %s\n", AppTitle, Version)
		return
	}

	// For setup, we need config loaded synchronously
	if *setup {
		conf, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
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

	// TIER 1 PERFORMANCE: Concurrent gathering
	type result struct {
		conf *config.Config
		info *git.Info
		err  error
	}
	confChan := make(chan result, 1)
	infoChan := make(chan result, 1)

	go func() {
		c, err := config.Load()
		confChan <- result{conf: c, err: err}
	}()

	go func() {
		// We use a large default (32KB) for the concurrent gather, 
		// as we don't have the config yet.
		i, err := git.GatherInfo(32000)
		infoChan <- result{info: i, err: err}
	}()

	resConf := <-confChan
	if resConf.err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", resConf.err)
		os.Exit(1)
	}
	resInfo := <-infoChan
	if resInfo.err != nil {
		fmt.Fprintf(os.Stderr, "Error gathering git info: %v\n", resInfo.err)
		os.Exit(1)
	}

	if err := runAnalyzer(commitMsgFile, resConf.conf, resInfo.info); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runAnalyzer orchestrates the commit message generation process.
// It uses pre-gathered repository information and the loaded config to generate a message via LLM.
func runAnalyzer(file string, conf *config.Config, info *git.Info) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(conf.TimeoutSeconds)*time.Second)
	defer cancel()

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
	case "anthropic":
		p, err := llm.NewAnthropic(pc.APIKey, pc.Model)
		if err != nil {
			return err
		}
		provider = p
	default:
		return fmt.Errorf("unsupported provider: %s", conf.ActiveProvider)
	}

	prompt := buildPrompt(info)
	fmt.Fprintf(os.Stderr, "Generating commit message via %s (%s)...\n", provider.Name(), pc.Model)

	msg, err := llm.GenerateWithRetry(ctx, provider, prompt, conf.RetryCount, time.Duration(conf.RetryDelaySeconds)*time.Second)
	if err != nil {
		return err
	}

	msg = cleanLLMOutput(msg)
	if len(msg) < 5 {
		return fmt.Errorf("AI message too short or empty after cleaning")
	}

	return writeMessage(file, msg, info)
}

// cleanLLMOutput strips unwanted conversational filler, markdown code fences,
// and leading/trailing whitespace from the LLM-generated text to ensure
// only the commit message remains.
func cleanLLMOutput(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(out))
	
	// Skip markdown code fences if present
	inFence := false
	firstLine := true
	
	for scanner.Scan() {
		line := scanner.Text()
		tl := strings.TrimSpace(line)
		
		if strings.HasPrefix(tl, "```") {
			inFence = !inFence
			continue
		}
		
		if inFence || (!strings.HasPrefix(tl, "Based on") && 
		   !strings.HasPrefix(tl, "Generate") && 
		   !strings.HasPrefix(tl, "Here is") && 
		   tl != "---") {
			if tl == "" && firstLine {
				continue
			}
			if !firstLine {
				sb.WriteString("\n")
			}
			sb.WriteString(line)
			firstLine = false
		}
	}
	
	return strings.TrimSpace(sb.String())
}

// buildPrompt constructs a detailed string prompt for the LLM, containing
// staged file names, change statistics, and the unified diff.
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

// writeMessage writes the generated commit message to the specified path,
// appending metadata about the changes as comments and preserving any
// existing content after the generated message.
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
