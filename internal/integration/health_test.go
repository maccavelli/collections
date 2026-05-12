package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"mcp-server-magicdev/internal/db"
)

func TestVerifyConnectivityNoToken(t *testing.T) {
	viper.Set("server.db_path", filepath.Join(os.TempDir(), "test_health.db"))
	store, _ := db.InitStore()
	defer store.Close()
	defer os.Remove(filepath.Join(os.TempDir(), "test_health.db"))

	store.SetSecret("gitlab", "")
	store.SetSecret("git", "")

	err := VerifyConnectivity(store)
	if err == nil {
		t.Error("Expected error for missing gitlab token")
	}
}

func TestVerifyConnectivityWithMocks(t *testing.T) {
	viper.Set("server.db_path", filepath.Join(os.TempDir(), "test_health2.db"))
	store, _ := db.InitStore()
	defer store.Close()
	defer os.Remove(filepath.Join(os.TempDir(), "test_health2.db"))

	store.SetSecret("gitlab", "test-token")
	viper.Set("git.server_url", "http://127.0.0.1:0") // Invalid URL to trigger failure

	err := VerifyConnectivity(store)
	if err == nil {
		t.Error("Expected error for bad gitlab connection")
	}

	viper.Set("jira.mock", true)
	viper.Set("confluence.mock", true)

	// Since we can't easily mock gitlab to return success without spinning up a server,
	// testing the failure case is sufficient for coverage.
}

func TestVerifyConnectivityJiraNoToken(t *testing.T) {
	viper.Set("git.mock", true)
	viper.Set("jira.mock", false)
	viper.Set("jira.url", "http://example.com")
	
	store, _ := db.InitStore()
	defer store.Close()
	
	err := VerifyConnectivity(store)
	if err == nil {
		t.Error("Expected error for missing jira token")
	}
}

func TestVerifyConnectivityConfluenceNoToken(t *testing.T) {
	viper.Set("git.mock", true)
	viper.Set("jira.mock", true)
	viper.Set("confluence.mock", false)
	viper.Set("confluence.url", "http://example.com")
	
	store, _ := db.InitStore()
	defer store.Close()
	
	err := VerifyConnectivity(store)
	if err == nil {
		t.Error("Expected error for missing confluence token")
	}
}

func TestVerifyConnectivitySuccess(t *testing.T) {
	viper.Set("git.mock", true)
	viper.Set("jira.mock", true)
	viper.Set("confluence.mock", true)
	
	store, _ := db.InitStore()
	defer store.Close()
	
	err := VerifyConnectivity(store)
	if err != nil {
		t.Errorf("Expected success, got %v", err)
	}
}
