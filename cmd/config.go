package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/llm"
)

// ConfigSyncFunc is a callback for the config sync logic
var ConfigSyncFunc func(configPath string) error

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage orchestrator configuration",
}

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Interactive setup wizard for the Semantic Intelligence LLM",
	RunE:  runConfigure,
}

// configureRootCmd is an alias so `mcp-server-magictools configure` works directly
var configureRootCmd = &cobra.Command{
	Use:   "configure",
	Short: "Interactive setup wizard for the Semantic Intelligence LLM",
	RunE:  runConfigure,
}

func runConfigure(cmd *cobra.Command, args []string) error {
	cfg, err := config.New(Version, CfgPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== MagicTools Intelligence Configuration ===")
	fmt.Println()
	fmt.Println("Which LLM Provider would you like to use for Semantic Hydration?")
	fmt.Println("  1) gemini    (Google Gemini API)")
	fmt.Println("  2) anthropic (Anthropic Claude API)")
	fmt.Println("  3) openai    (OpenAI API)")
	fmt.Print("\nEnter choice (1-3): ")

	choiceStr, _ := reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)

	var provider string
	switch choiceStr {
	case "1":
		provider = "gemini"
	case "2":
		provider = "anthropic"
	case "3":
		provider = "openai"
	default:
		fmt.Println("Invalid choice. Exiting.")
		return nil
	}

	// 🛡️ ENV VAR AUTO-DETECTION: Check for API key in environment first
	var apiKey string
	envVars := map[string]string{
		"gemini":    "GEMINI_API_KEY",
		"openai":    "OPENAI_API_KEY",
		"anthropic": "ANTHROPIC_API_KEY",
	}

	if envVar, ok := envVars[provider]; ok {
		if val := os.Getenv(envVar); val != "" {
			fmt.Printf("\n✓ Detected %s in environment.\n", envVar)
			fmt.Print("Press Enter to use it, or type a different key to override: ")
			override, _ := reader.ReadString('\n')
			override = strings.TrimSpace(override)
			if override != "" {
				apiKey = override
				fmt.Println("Using manually entered key.")
			} else {
				apiKey = val
				fmt.Println("Using environment key.")
			}
		}
	}

	if apiKey == "" {
		fmt.Printf("\nEnter your %s API Key: ", provider)
		keyStr, _ := reader.ReadString('\n')
		apiKey = strings.TrimSpace(keyStr)
	}

	if apiKey == "" {
		fmt.Println("No API key provided. Exiting.")
		return nil
	}

	// 🔍 MODEL DISCOVERY: Attempt dynamic model listing from the provider
	fmt.Println("\nDiscovering available models...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var models []string
	var discoverer any
	switch provider {
	case "gemini":
		discoverer, _ = llm.NewGemini(ctx, apiKey, "")
	case "openai":
		discoverer = llm.NewOpenAI(apiKey, "")
	case "anthropic":
		discoverer, _ = llm.NewAnthropic(apiKey, "")
	}

	if md, ok := discoverer.(llm.ModelDiscoverer); ok {
		discovered, err := md.DiscoverModels(ctx)
		if err == nil && len(discovered) > 0 {
			models = discovered
			fmt.Printf("✓ Found %d models from %s.\n", len(models), provider)
		} else {
			fmt.Printf("⚠ Model discovery failed: %v. Using defaults.\n", err)
		}
	}

	// Fallback to hardcoded model lists
	if len(models) == 0 {
		switch provider {
		case "gemini":
			models = []string{"gemini-2.0-flash-lite", "gemini-2.0-flash", "gemini-1.5-flash"}
		case "openai":
			models = []string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"}
		case "anthropic":
			models = []string{"claude-3-5-haiku-latest", "claude-3-5-sonnet-latest"}
		}
	}

	fmt.Println("\nAvailable models:")
	for i, m := range models {
		fmt.Printf("  %d) %s\n", i+1, m)
	}
	fmt.Printf("  %d) Other (enter manually)\n", len(models)+1)
	fmt.Print("\nSelect model: ")

	modelChoice, _ := reader.ReadString('\n')
	modelChoice = strings.TrimSpace(modelChoice)

	var selectedModel string
	idx := 0
	if _, err := fmt.Sscanf(modelChoice, "%d", &idx); err == nil && idx >= 1 && idx <= len(models) {
		selectedModel = models[idx-1]
	} else if idx == len(models)+1 {
		fmt.Print("Enter model name: ")
		custom, _ := reader.ReadString('\n')
		selectedModel = strings.TrimSpace(custom)
	} else {
		// Try as raw string
		selectedModel = modelChoice
	}

	if selectedModel == "" {
		fmt.Println("No model selected. Exiting.")
		return nil
	}

	// 🔄 FALLBACK MODELS: All non-selected models become fallbacks
	var fallbacks []string
	for _, m := range models {
		if m != selectedModel {
			fallbacks = append(fallbacks, m)
		}
	}

	cfg.Intelligence.Provider = provider
	cfg.Intelligence.APIKey = apiKey
	cfg.Intelligence.Model = selectedModel
	cfg.Intelligence.FallbackModels = fallbacks

	// Set defaults for retry/timeout if not already configured
	if cfg.Intelligence.RetryCount <= 0 {
		cfg.Intelligence.RetryCount = 2
	}
	if cfg.Intelligence.RetryDelay <= 0 {
		cfg.Intelligence.RetryDelay = 5
	}
	if cfg.Intelligence.TimeoutSeconds <= 0 {
		cfg.Intelligence.TimeoutSeconds = 120
	}

	if err := cfg.SaveConfiguration(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("\n✓ Intelligence Engine configured!\n")
	fmt.Printf("  Provider:  %s\n", provider)
	fmt.Printf("  Model:     %s\n", selectedModel)
	if len(fallbacks) > 0 {
		fmt.Printf("  Fallbacks: %s\n", strings.Join(fallbacks, ", "))
	}
	fmt.Printf("  Retries:   %d (delay: %ds)\n", cfg.Intelligence.RetryCount, cfg.Intelligence.RetryDelay)
	fmt.Printf("  Timeout:   %ds\n", cfg.Intelligence.TimeoutSeconds)

	return nil
}

func init() {
	configCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(configureRootCmd) // Root alias: `mcp-server-magictools configure`
}
