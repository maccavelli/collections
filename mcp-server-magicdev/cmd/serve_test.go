package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"mcp-server-magicdev/internal/db"
)

func TestProvisionVault(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test_provision_vault.db")
	defer os.Remove(dbPath)

	store, err := db.InitStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	os.Setenv("TEST_PROVISION_ENV", "test-secret-value")
	defer os.Unsetenv("TEST_PROVISION_ENV")

	provisionVault(store, "test-service", "TEST_PROVISION_ENV")

	val, err := store.GetSecret("test-service")
	if err != nil {
		t.Errorf("Unexpected error getting secret: %v", err)
	}
	if val != "test-secret-value" {
		t.Errorf("Expected 'test-secret-value', got '%v'", val)
	}
}

func TestCheckVaultSecret(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "test_check_vault.db")
	defer os.Remove(dbPath)

	store, err := db.InitStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	// Should just log a warning and not panic
	checkVaultSecret(store, "missing-service")

	store.SetSecret("existing-service", "secret")
	// Should do nothing and not panic
	checkVaultSecret(store, "existing-service")
}
