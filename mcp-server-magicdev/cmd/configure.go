// Package cmd implements the Cobra command tree for the MagicDev MCP server.
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"mcp-server-magicdev/internal/config"
	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/integration/llm"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Interactive setup menu for MagicDev configurations and tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Ensure config exists
		_, err := config.EnsureConfig()
		if err != nil {
			return fmt.Errorf("failed to ensure config: %w", err)
		}

		store, err := db.InitStore()
		if err != nil {
			return fmt.Errorf("failed to initialize BuntDB vault: %w", err)
		}
		defer store.Close()

		reader := bufio.NewReader(os.Stdin)

		for {
			fmt.Println("\n=== MagicDev Configuration Menu ===")
			fmt.Println("1. Setup Jira")
			fmt.Println("2. Setup Confluence")
			fmt.Println("3. Setup Gitlab")
			fmt.Println("4. Setup LLM")
			fmt.Println("0. Exit")
			fmt.Print("\nSelect an option: ")

			optStr, _ := reader.ReadString('\n')
			optStr = strings.TrimSpace(optStr)

			switch optStr {
			case "1":
				setupJira(reader, store)
			case "2":
				setupConfluence(reader, store)
			case "3":
				setupGitlab(reader, store)
			case "4":
				setupLLM(reader, store)
			case "0":
				fmt.Println("Exiting configuration menu.")
				return nil
			default:
				fmt.Println("Invalid option. Please try again.")
			}
		}
	},
}

func setupJira(reader *bufio.Reader, store *db.Store) {
	fmt.Println("\n--- Setup Jira ---")
	fmt.Print("Email Address: ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)

	fmt.Print("Token: ")
	token, _ := reader.ReadString('\n')
	token = strings.TrimSpace(token)

	fmt.Print("URL (e.g. https://your-domain.atlassian.net): ")
	urlStr, _ := reader.ReadString('\n')
	urlStr = strings.TrimSpace(urlStr)

	if email != "" {
		_ = config.UpdateConfigKey("jira.email", email)
	}
	if urlStr != "" {
		_ = config.UpdateConfigKey("jira.url", urlStr)
	}
	if token != "" {
		_ = store.SetSecret("jira", token)
	}
	fmt.Println("Jira configuration saved.")
}

func setupConfluence(reader *bufio.Reader, store *db.Store) {
	fmt.Println("\n--- Setup Confluence ---")
	fmt.Print("Token: ")
	token, _ := reader.ReadString('\n')
	token = strings.TrimSpace(token)

	fmt.Print("URL (e.g. https://your-domain.atlassian.net/wiki): ")
	urlStr, _ := reader.ReadString('\n')
	urlStr = strings.TrimSpace(urlStr)

	if urlStr != "" {
		_ = config.UpdateConfigKey("confluence.url", urlStr)
	}
	if token != "" {
		_ = store.SetSecret("confluence", token)
	}
	fmt.Println("Confluence configuration saved.")
}

func setupGitlab(reader *bufio.Reader, store *db.Store) {
	fmt.Println("\n--- Setup Gitlab ---")
	fmt.Print("Username: ")
	user, _ := reader.ReadString('\n')
	user = strings.TrimSpace(user)

	fmt.Print("Token: ")
	token, _ := reader.ReadString('\n')
	token = strings.TrimSpace(token)

	if user != "" {
		_ = config.UpdateConfigKey("git.username", user)
	}
	if token != "" {
		_ = store.SetSecret("gitlab", token)
	}
	fmt.Println("Gitlab configuration saved.")
}

func setupLLM(reader *bufio.Reader, store *db.Store) {
	fmt.Println("\n--- Setup LLM ---")
	fmt.Println("1. Gemini")
	fmt.Println("2. OpenAI")
	fmt.Println("3. Claude")
	fmt.Println("0. Cancel")
	fmt.Print("Select Provider: ")

	pStr, _ := reader.ReadString('\n')
	pStr = strings.TrimSpace(pStr)

	var provider llm.Provider
	switch pStr {
	case "1":
		provider = llm.ProviderGemini
	case "2":
		provider = llm.ProviderOpenAI
	case "3":
		provider = llm.ProviderClaude
	case "0":
		return
	default:
		fmt.Println("Invalid provider.")
		return
	}

	fmt.Print("API Key: ")
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)

	if apiKey == "" {
		fmt.Println("API Key is required.")
		return
	}

	fmt.Println("Fetching available models...")
	models, err := llm.ListModels(context.Background(), provider, apiKey)
	if err != nil {
		fmt.Printf("Failed to fetch models: %v\n", err)
		return
	}

	if len(models) == 0 {
		fmt.Println("No models found.")
		return
	}

	fmt.Println("\nAvailable Models:")
	for i, m := range models {
		fmt.Printf("%d. %s\n", i+1, m)
	}

	fmt.Print("\nSelect Default Model (number): ")
	mIdxStr, _ := reader.ReadString('\n')
	mIdxStr = strings.TrimSpace(mIdxStr)

	idx, err := strconv.Atoi(mIdxStr)
	if err != nil || idx < 1 || idx > len(models) {
		fmt.Println("Invalid model selection.")
		return
	}

	selectedModel := models[idx-1]

	_ = store.SetSecret("llm_provider", string(provider))
	_ = config.UpdateConfigKey("llm.model", selectedModel)
	_ = store.SetSecret("llm_token", apiKey)

	fmt.Printf("LLM configuration saved. Default model: %s\n", selectedModel)
}

func init() {
	rootCmd.AddCommand(configureCmd)
}
