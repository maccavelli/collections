package llm

import (
	"context"
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient(ProviderOpenAI, "key", "model")
	if client == nil {
		t.Error("expected non-nil client")
	}
}

func TestListModelsUnsupported(t *testing.T) {
	_, err := ListModels(context.Background(), Provider("unknown"), "key")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestGenerateContentUnsupported(t *testing.T) {
	client := NewClient(Provider("unknown"), "key", "model")
	_, err := client.GenerateContent(context.Background(), "prompt")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestListModels_OpenAI_Error(t *testing.T) {
	_, err := ListModels(context.Background(), ProviderOpenAI, "invalid-key")
	if err == nil {
		t.Error("expected error for invalid openai key")
	}
}

func TestListModels_Claude_Error(t *testing.T) {
	_, err := ListModels(context.Background(), ProviderClaude, "invalid-key")
	if err == nil {
		t.Error("expected error for invalid claude key")
	}
}

func TestListModels_Gemini_Error(t *testing.T) {
	_, err := ListModels(context.Background(), ProviderGemini, "invalid-key")
	if err == nil {
		t.Error("expected error for invalid gemini key")
	}
}

func TestGenerateContent_OpenAI_Error(t *testing.T) {
	client := NewClient(ProviderOpenAI, "invalid-key", "gpt-3.5-turbo")
	_, err := client.GenerateContent(context.Background(), "prompt")
	if err == nil {
		t.Error("expected error for invalid openai key")
	}
}

func TestGenerateContent_Claude_Error(t *testing.T) {
	client := NewClient(ProviderClaude, "invalid-key", "claude-3-haiku-20240307")
	_, err := client.GenerateContent(context.Background(), "prompt")
	if err == nil {
		t.Error("expected error for invalid claude key")
	}
}

func TestGenerateContent_Gemini_Error(t *testing.T) {
	client := NewClient(ProviderGemini, "invalid-key", "gemini-1.5-flash")
	_, err := client.GenerateContent(context.Background(), "prompt")
	if err == nil {
		t.Error("expected error for invalid gemini key")
	}
}

func TestJSONResponse_Error(t *testing.T) {
	// Should fail because provider is unknown
	client := NewClient(Provider("unknown"), "key", "model")
	var target map[string]interface{}
	err := client.JSONResponse(context.Background(), "prompt", &target)
	if err == nil {
		t.Error("expected error for unknown provider in JSONResponse")
	}
}
