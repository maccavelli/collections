// Package integration provides external service connectivity and workflow integration.
package integration

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/integration/llm"
)

// ErrLLMDisabled is returned when the Intelligence Engine is explicitly disabled via configuration.
var ErrLLMDisabled = errors.New("llm disabled by configuration")

// NewLLMClient initializes the unified LLM client from config and BuntDB vault.
// Provider and model are read from viper (magicdev.yaml), with a backward-compatible
// fallback to the BuntDB vault for provider if the config value is empty.
// The API token is always read from the vault.
func NewLLMClient(store *db.Store) (*llm.Client, error) {
	if viper.GetBool("llm.disable") {
		return nil, ErrLLMDisabled
	}

	// Provider: config-first, vault fallback for pre-migration users.
	providerStr := viper.GetString("llm.provider")
	if providerStr == "" {
		providerStr, _ = store.GetSecret("llm_provider")
	}
	if providerStr == "" {
		return nil, fmt.Errorf("llm provider not configured")
	}

	model := viper.GetString("llm.model")
	if model == "" {
		return nil, fmt.Errorf("llm model not configured")
	}

	token, err := store.GetSecret("llm_token")
	if err != nil || token == "" {
		return nil, fmt.Errorf("llm token not configured")
	}

	var provider llm.Provider
	switch strings.ToLower(providerStr) {
	case "gemini":
		provider = llm.ProviderGemini
	case "openai":
		provider = llm.ProviderOpenAI
	case "claude":
		provider = llm.ProviderClaude
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", providerStr)
	}

	return llm.NewClient(provider, token, model), nil
}
