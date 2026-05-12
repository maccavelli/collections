package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	"mcp-server-magicdev/internal/db"
)

func TestNewLLMClient(t *testing.T) {
	viper.Set("server.db_path", filepath.Join(os.TempDir(), "test_llm.db"))
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()
	defer os.Remove(filepath.Join(os.TempDir(), "test_llm.db"))

	store.SetSecret("llm_provider", "openai")
	store.SetSecret("llm_token", "test-token")
	viper.Set("llm.model", "test-model")

	client, err := NewLLMClient(store)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if client == nil {
		t.Error("expected non-nil client")
	}

	// Test missing provider
	store.SetSecret("llm_provider", "")
	_, err = NewLLMClient(store)
	if err == nil {
		t.Error("expected error for missing provider")
	}
}
