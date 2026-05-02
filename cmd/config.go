package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"mcp-server-magictools/internal/config"
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
	fmt.Println("What would you like to configure?")
	fmt.Println("  1) Standard Search Model (Generative LLMs)")
	fmt.Println("  2) Vector Search Model (Embedding Engines)")
	fmt.Print("\nEnter choice (1-2): ")

	modeStr, _ := reader.ReadString('\n')
	modeStr = strings.TrimSpace(modeStr)

	if modeStr == "1" {
		return configureStandardSearch(cfg, reader)
	} else if modeStr == "2" {
		return configureVectorSearch(cfg, reader)
	} else {
		fmt.Println("Invalid choice. Exiting.")
		return nil
	}
}

func configureStandardSearch(cfg *config.Config, reader *bufio.Reader) error {
	fmt.Println("\nWhich LLM Provider would you like to use for Semantic Hydration?")
	fmt.Println("  1) gemini    (Google Gemini API)")
	fmt.Println("  2) anthropic (Anthropic Claude API)")
	fmt.Println("  3) openai    (OpenAI API)")
	fmt.Print("\nEnter choice (1-3): ")

	choiceStr, _ := reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)

	var provider string
	var models []string
	switch choiceStr {
	case "1":
		provider = "gemini"
		models = []string{"gemini-2.5-flash", "gemini-2.0-flash", "gemini-2.0-flash-lite", "gemini-1.5-pro"}
	case "2":
		provider = "anthropic"
		models = []string{"claude-3-7-sonnet-latest", "claude-3-5-sonnet-latest", "claude-3-5-haiku-latest"}
	case "3":
		provider = "openai"
		models = []string{"gpt-4o", "gpt-4o-mini", "o3-mini", "o1-mini"}
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
		selectedModel = modelChoice
	}

	if selectedModel == "" {
		fmt.Println("No model selected. Exiting.")
		return nil
	}

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

	fmt.Printf("\n✓ Standard Intelligence Engine configured!\n")
	fmt.Printf("  Provider:  %s\n", provider)
	fmt.Printf("  Model:     %s\n", selectedModel)
	if len(fallbacks) > 0 {
		fmt.Printf("  Fallbacks: %s\n", strings.Join(fallbacks, ", "))
	}
	return nil
}

func configureVectorSearch(cfg *config.Config, reader *bufio.Reader) error {
	fmt.Println("\nWhich Provider would you like to use for Vector Embeddings?")
	fmt.Println("  1) gemini (Google Gemini API)")
	fmt.Println("  2) voyage (Claude Embeddings via Voyage API)")
	fmt.Println("  3) openai (OpenAI API)")
	fmt.Println("  4) ollama (Local API)")
	fmt.Print("\nEnter choice (1-4): ")

	choiceStr, _ := reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)

	var provider string
	var models []string
	var dimsMap map[string]int

	switch choiceStr {
	case "1":
		provider = "gemini"
		models = []string{"text-embedding-004 (256 dims)", "text-embedding-004 (768 dims)"}
		dimsMap = map[string]int{
			"text-embedding-004 (256 dims)": 256,
			"text-embedding-004 (768 dims)": 768,
		}
	case "2":
		provider = "voyage"
		models = []string{"voyage-3-lite", "voyage-3", "voyage-code-3"}
		dimsMap = map[string]int{
			"voyage-3-lite": 512,
			"voyage-3":      1024,
			"voyage-code-3": 1024,
		}
	case "3":
		provider = "openai"
		models = []string{"text-embedding-3-small (512 dims)", "text-embedding-3-small (1536 dims)", "text-embedding-3-large (256 dims)", "text-embedding-3-large (1024 dims)"}
		dimsMap = map[string]int{
			"text-embedding-3-small (512 dims)":  512,
			"text-embedding-3-small (1536 dims)": 1536,
			"text-embedding-3-large (256 dims)":  256,
			"text-embedding-3-large (1024 dims)": 1024,
		}
	case "4":
		provider = "ollama"
		models = []string{"granite-embedding:30m", "snowflake-arctic-embed:33m", "all-minilm:33m", "nomic-embed-text"}
		dimsMap = map[string]int{
			"granite-embedding:30m":      384,
			"snowflake-arctic-embed:33m": 384,
			"all-minilm:33m":             384,
			"nomic-embed-text":           768,
		}
	default:
		fmt.Println("Invalid choice. Exiting.")
		return nil
	}

	var apiKey string
	var apiURL string
	if provider == "ollama" {
		fmt.Print("\nEnter Ollama API URL (default: http://localhost:11434): ")
		apiURL, _ = reader.ReadString('\n')
		apiURL = strings.TrimSpace(apiURL)
		if apiURL == "" {
			apiURL = "http://localhost:11434"
		}
	} else {
		envVars := map[string]string{
			"gemini": "GEMINI_API_KEY",
			"openai": "OPENAI_API_KEY",
			"voyage": "VOYAGE_API_KEY",
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
	}

	fmt.Println("\nAvailable models:")
	for i, m := range models {
		fmt.Printf("  %d) %s\n", i+1, m)
	}
	fmt.Printf("  %d) Other (enter manually)\n", len(models)+1)
	fmt.Print("\nSelect model: ")

	modelChoice, _ := reader.ReadString('\n')
	modelChoice = strings.TrimSpace(modelChoice)

	var selectedDisplay string
	idx := 0
	if _, err := fmt.Sscanf(modelChoice, "%d", &idx); err == nil && idx >= 1 && idx <= len(models) {
		selectedDisplay = models[idx-1]
	} else if idx == len(models)+1 {
		fmt.Print("Enter model name: ")
		custom, _ := reader.ReadString('\n')
		selectedDisplay = strings.TrimSpace(custom)
	} else {
		selectedDisplay = modelChoice
	}

	if selectedDisplay == "" {
		fmt.Println("No model selected. Exiting.")
		return nil
	}

	dims := dimsMap[selectedDisplay]
	actualModel := strings.Split(selectedDisplay, " ")[0] // Strip " (256 dims)"

	if dims == 0 {
		fmt.Print("Enter dimensionality for custom model (e.g. 384, 768, 1536): ")
		dimStr, _ := reader.ReadString('\n')
		fmt.Sscanf(strings.TrimSpace(dimStr), "%d", &dims)
	}

	cfg.Intelligence.EmbeddingProvider = provider
	cfg.Intelligence.EmbeddingModel = actualModel
	cfg.Intelligence.EmbeddingAPIKey = apiKey
	if provider == "ollama" {
		cfg.Intelligence.EmbeddingAPIURL = apiURL
	}
	cfg.Intelligence.EmbeddingDimensionality = dims
	cfg.Intelligence.VectorEnabled = true

	if err := cfg.SaveConfiguration(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("\n✓ Vector Engine configured!\n")
	fmt.Printf("  Provider:   %s\n", provider)
	fmt.Printf("  Model:      %s\n", actualModel)
	fmt.Printf("  Dimensions: %d\n", dims)
	fmt.Printf("  Enabled:    %v\n", cfg.Intelligence.VectorEnabled)
	return nil
}

func init() {
	configCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(configureRootCmd) // Root alias: `mcp-server-magictools configure`
}
