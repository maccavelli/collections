package llm

import (
	"context"
)

// Provider defines the interface for LLM backends that generate commit messages.
type Provider interface {
	// Name returns the display name of the provider (e.g., "openai", "gemini").
	Name() string
	// Generate sends a prompt to the LLM and returns the generated text.
	Generate(ctx context.Context, prompt string) (string, error)
}
