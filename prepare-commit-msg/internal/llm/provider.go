package llm

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Provider defines the interface for LLM backends that generate commit messages.
type Provider interface {
	// Name returns the display name of the provider (e.g., "openai", "gemini").
	Name() string
	// Generate sends a prompt to the LLM and returns the generated text.
	Generate(ctx context.Context, prompt string) (string, error)
}

// GenerateWithRetry executes a Generate call with the specified number of retries
// and delay. It will stop Retrying if the context is cancelled.
func GenerateWithRetry(ctx context.Context, p Provider, prompt string, retries int, delay time.Duration) (string, error) {
	var lastErr error
	for i := 0; i <= retries; i++ {
		if i > 0 {
			fmt.Fprintf(os.Stderr, "LLM request failed (Attempt %d/%d). Retrying in %v...\n", i, retries+1, delay)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
				// Ready for next attempt
			}
		}

		res, err := p.Generate(ctx, prompt)
		if err == nil {
			return res, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("failed after %d attempts: %w", retries+1, lastErr)
}
