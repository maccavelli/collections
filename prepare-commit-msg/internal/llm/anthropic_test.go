package llm

import (
	"strings"
	"testing"
)

func TestAnthropicProvider_Name(t *testing.T) {
	p, _ := NewAnthropic("fake-key", "claude-3-5-haiku-latest")
	if p.Name() != "anthropic" {
		t.Errorf("expected anthropic, got %s", p.Name())
	}
}

func TestNewAnthropic_Error(t *testing.T) {
	_, err := NewAnthropic("", "model")
	if err == nil {
		t.Error("expected error for empty api key")
	}
	if !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// Note: Full Generate testing would require mocking the anthropic.Client,
// which is complex due to the external library structure. 
// We verify the provider initialization and interface compliance here.
func TestAnthropicProvider_Interface(t *testing.T) {
	var _ Provider = (*AnthropicProvider)(nil)
}
