package ui

import (
	"context"
	"fmt"
	"os"

	"prepare-commit-msg/internal/config"
	"prepare-commit-msg/internal/llm"

	"github.com/AlecAivazis/survey/v2"
)

// RunSetup runs the interactive configuration wizard for provider and model selection.
func RunSetup(ctx context.Context, conf *config.Config) error {
	fmt.Println("--- prepare-commit-msg Setup ---")

	var provider string
	prompt := &survey.Select{
		Message: "Choose LLM Provider:",
		Options: []string{"gemini", "openai", "anthropic"},
	}
	if err := survey.AskOne(prompt, &provider); err != nil {
		return err
	}

	pc, ok := conf.Providers[provider]
	if !ok {
		pc = config.ProviderConfig{}
	}

	var apiKey string
	envVar := ""
	switch provider {
	case "gemini":
		envVar = "GEMINI_API_KEY"
	case "openai":
		envVar = "OPENAI_API_KEY"
	case "anthropic":
		envVar = "ANTHROPIC_API_KEY"
	}

	if envVar != "" {
		if val := os.Getenv(envVar); val != "" {
			fmt.Printf("%s detected in environment! Importing API key...\n", envVar)
			apiKey = val
		}
	}

	if apiKey == "" {
		if err := survey.AskOne(&survey.Password{
			Message: fmt.Sprintf("Enter %s API Key:", provider),
		}, &apiKey); err != nil {
			return err
		}
	}

	if apiKey != "" {
		pc.APIKey = apiKey
	}

	var models []string
	// TIER 3 PERFORMANCE: Attempt dynamic discovery if the provider supports it.
	var discoverer any
	switch provider {
	case "gemini":
		discoverer, _ = llm.NewGemini(ctx, pc.APIKey, "")
	case "openai":
		discoverer = llm.NewOpenAI(pc.APIKey, "")
	case "anthropic":
		discoverer, _ = llm.NewAnthropic(pc.APIKey, "")
	}

	if md, ok := discoverer.(llm.ModelDiscoverer); ok {
		fmt.Println("I am currently finding the best models, please hang tight!")
		discovered, err := md.DiscoverModels(ctx)
		if err == nil && len(discovered) > 0 {
			models = discovered
		}
	}

	if len(models) == 0 {
		if provider == "openai" {
			models = []string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"}
		} else if provider == "anthropic" {
			models = []string{"claude-3-5-haiku-latest", "claude-3-5-sonnet-latest"}
		} else {
			models = []string{"gemini-2.0-flash-lite", "gemini-2.0-flash", "gemini-1.5-flash"}
		}
	}
	models = append(models, "Other")

	var model string
	if err := survey.AskOne(&survey.Select{
		Message: "Select Model:",
		Options: models,
	}, &model); err != nil {
		return err
	}

	if model == "Other" {
		if err := survey.AskOne(&survey.Input{
			Message: "Enter model name:",
		}, &model); err != nil {
			return err
		}
	}
	pc.Model = model

	conf.ActiveProvider = provider
	conf.Providers[provider] = pc

	if err := conf.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\nConfiguration saved! Active provider: %s (%s)\n", provider, pc.Model)
	return nil
}
