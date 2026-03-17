package llm

import (
	"context"
)

type Provider interface {
	Name() string
	Generate(ctx context.Context, prompt string) (string, error)
}

type Config struct {
	APIKey string
	Model  string
}
