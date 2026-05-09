// Package integration provides external service connectivity and workflow integration.
package integration

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/integration/llm"
)

// NewLLMClient initializes the unified LLM client securely from BuntDB.
func NewLLMClient(store *db.Store) (*llm.Client, error) {
	providerStr, err := store.GetSecret("llm_provider")
	if err != nil || providerStr == "" {
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
