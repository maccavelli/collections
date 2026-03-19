package ui

import (
	"fmt"
	"os"

	"prepare-commit-msg/internal/config"

	"github.com/AlecAivazis/survey/v2"
)

// RunSetup runs the interactive configuration wizard for provider and model selection.
func RunSetup(conf *config.Config) error {
	fmt.Println("--- prepare-commit-msg Setup ---")

	var provider string
	prompt := &survey.Select{
		Message: "Choose LLM Provider:",
		Options: []string{"gemini", "openai"},
	}
	if err := survey.AskOne(prompt, &provider); err != nil {
		return err
	}

	pc := conf.Providers[provider]

	var apiKey string
	envVar := ""
	switch provider {
	case "gemini":
		envVar = "GEMINI_API_KEY"
	case "openai":
		envVar = "OPENAI_API_KEY"
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
	if provider == "openai" {
		models = []string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo", "Other"}
	} else {
		models = []string{"gemini-2.5-flash-lite", "gemini-2.5-flash", "gemini-2.5-pro", "Other"}
	}

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
