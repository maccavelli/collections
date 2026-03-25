package llm

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Generate(ctx context.Context, prompt string) (string, error) {
	if strings.Contains(prompt, "empty") {
		return "", nil
	}
	return "mocked result", nil
}

func TestMockProvider_Name(t *testing.T) {
	m := &mockProvider{}
	if m.Name() != "mock" {
		t.Errorf("expected name mock, got %s", m.Name())
	}
}

func TestMockProvider_Generate(t *testing.T) {
	m := &mockProvider{}
	ctx := context.Background()

	got, err := m.Generate(ctx, "hello")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if got != "mocked result" {
		t.Errorf("expected mocked result, got %s", got)
	}
}

func TestGenerateWithRetry(t *testing.T) {
	ctx := context.Background()
	m := &mockProvider{}

	t.Run("success first try", func(t *testing.T) {
		got, err := GenerateWithRetry(ctx, m, "hello", 3, 0)
		if err != nil {
			t.Errorf("expected success, got %v", err)
		}
		if got != "mocked result" {
			t.Errorf("expected mocked result, got %s", got)
		}
	})

	t.Run("retry on error until success", func(t *testing.T) {
		failCount := 0
		failThenSuccess := &mockFailProvider{
			failUntil: 2,
			OnFail:    func() { failCount++ },
		}
		got, err := GenerateWithRetry(ctx, failThenSuccess, "hello", 3, 0)
		if err != nil {
			t.Errorf("expected eventually successful, got %v", err)
		}
		if got != "eventual success" {
			t.Errorf("expected eventual success, got %s", got)
		}
		if failCount != 2 {
			t.Errorf("expected 2 failures before success, got %d", failCount)
		}
	})

	t.Run("exhaust retries", func(t *testing.T) {
		mFail := &mockFailProvider{failUntil: 5}
		_, err := GenerateWithRetry(ctx, mFail, "hello", 2, 0)
		if err == nil {
			t.Error("expected error after retries exhausted, got nil")
		}
	})
}

type mockFailProvider struct {
	failUntil int
	attempts  int
	OnFail    func()
}

func (m *mockFailProvider) Name() string { return "fail" }
func (m *mockFailProvider) Generate(ctx context.Context, prompt string) (string, error) {
	m.attempts++
	if m.attempts <= m.failUntil {
		if m.OnFail != nil {
			m.OnFail()
		}
		return "", fmt.Errorf("temporary failure")
	}
	return "eventual success", nil
}

// Note: TestNewGemini and TestNewOpenAI are skipped as they require actual clients/API keys or heavy mocking of external SDKs.
// The interface implementation is verified via the mock provider.
