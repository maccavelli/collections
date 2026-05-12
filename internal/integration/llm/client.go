// Package llm provides functionality for the llm subsystem.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/google/generative-ai-go/genai"
	"github.com/sashabaranov/go-openai"
	api_option "google.golang.org/api/option"
	"google.golang.org/api/iterator"
)

// Provider represents the LLM provider.
type Provider string

const (
	ProviderGemini Provider = "gemini"
	ProviderOpenAI Provider = "openai"
	ProviderClaude Provider = "claude"
)

// Client is the internal LLM client.
type Client struct {
	provider Provider
	apiKey   string
	model    string
}

// NewClient creates a new LLM client.
func NewClient(provider Provider, apiKey, model string) *Client {
	return &Client{
		provider: provider,
		apiKey:   apiKey,
		model:    model,
	}
}

// ListModels fetches the available models dynamically from the provider.
func ListModels(ctx context.Context, provider Provider, apiKey string) ([]string, error) {
	switch provider {
	case ProviderOpenAI:
		client := openai.NewClient(apiKey)
		models, err := client.ListModels(ctx)
		if err != nil {
			return nil, fmt.Errorf("openai list models error: %w", err)
		}
		var res []string
		for _, m := range models.Models {
			if strings.HasPrefix(m.ID, "gpt") {
				res = append(res, m.ID)
			}
		}
		return res, nil

	case ProviderClaude:
		client := anthropic.NewClient(option.WithAPIKey(apiKey))
		resData, err := client.Models.List(ctx, anthropic.ModelListParams{})
		if err != nil {
			return nil, fmt.Errorf("claude list models error: %w", err)
		}
		var res []string
		for _, m := range resData.Data {
			res = append(res, m.ID)
		}
		return res, nil

	case ProviderGemini:
		client, err := genai.NewClient(ctx, api_option.WithAPIKey(apiKey))
		if err != nil {
			return nil, fmt.Errorf("gemini client init error: %w", err)
		}
		defer client.Close()
		iter := client.ListModels(ctx)
		var res []string
		for {
			model, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("gemini list models error: %w", err)
			}
			res = append(res, strings.TrimPrefix(model.Name, "models/"))
		}
		return res, nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// GenerateContent sends a prompt to the configured LLM and returns the response.
func (c *Client) GenerateContent(ctx context.Context, prompt string) (string, error) {
	switch c.provider {
	case ProviderOpenAI:
		client := openai.NewClient(c.apiKey)
		resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: c.model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: prompt},
			},
		})
		if err != nil {
			return "", err
		}
		if len(resp.Choices) > 0 {
			return resp.Choices[0].Message.Content, nil
		}
		return "", fmt.Errorf("no response choices")

	case ProviderClaude:
		client := anthropic.NewClient(option.WithAPIKey(c.apiKey))
		resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(c.model),
			MaxTokens: int64(4096),
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			},
		})
		if err != nil {
			return "", err
		}
		if len(resp.Content) > 0 {
			return resp.Content[0].Text, nil
		}
		return "", fmt.Errorf("no response content")

	case ProviderGemini:
		client, err := genai.NewClient(ctx, api_option.WithAPIKey(c.apiKey))
		if err != nil {
			return "", err
		}
		defer client.Close()
		model := client.GenerativeModel(c.model)
		resp, err := model.GenerateContent(ctx, genai.Text(prompt))
		if err != nil {
			return "", err
		}
		if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
			if part, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
				return string(part), nil
			}
		}
		return "", fmt.Errorf("no response parts")

	default:
		return "", fmt.Errorf("unsupported provider: %s", c.provider)
	}
}

// JSONResponse generates a JSON payload using the LLM.
func (c *Client) JSONResponse(ctx context.Context, prompt string, target interface{}) error {
	prompt = prompt + "\n\nReturn strictly valid JSON without markdown wrapping or code blocks."
	resp, err := c.GenerateContent(ctx, prompt)
	if err != nil {
		return err
	}
	resp = strings.TrimPrefix(strings.TrimSpace(resp), "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	return json.Unmarshal([]byte(resp), target)
}
