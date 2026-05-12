package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magicdev/internal/config"
	"mcp-server-magicdev/internal/db"
)

func TestEvaluateIdeaHandler(t *testing.T) {
	tmpDB, _ := os.CreateTemp("", "testdb-*.db")
	defer os.Remove(tmpDB.Name())
	viper.Set("server.db_path", tmpDB.Name())
	viper.Set("git.mock", true)
	viper.Set("jira.mock", true)
	viper.Set("confluence.mock", true)

	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	handler := &ToolHandler{store: store}
	
	// Mock required vault tokens
	store.SetSecret("gitlab_token", "fake-token")
	store.SetSecret("jira_token", "fake-token")
	store.SetSecret("confluence_token", "fake-token")
	store.SetSecret("llm_api_key", "fake-token")

	args := EvaluateIdeaArgs{
		RawIdea:           "Test idea",
		TargetStack:       "Go",
		Labels:            []string{"backend"},
		TargetEnvironment: "cloud",
		BusinessCase:      "Test business case",
	}

	req := &mcp.CallToolRequest{}
	res, _, err := handler.EvaluateIdea(context.Background(), req, args)
	if err != nil {
		t.Fatalf("EvaluateIdea error: %v", err)
	}
	if res.IsError {
		msg := "unknown error"
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				msg = tc.Text
			}
		}
		t.Errorf("Expected success, got error: %s", msg)
	}
}

func TestCompleteDesignHandler(t *testing.T) {
	tmpDB, _ := os.CreateTemp("", "testdb-*.db")
	defer os.Remove(tmpDB.Name())
	viper.Set("server.db_path", tmpDB.Name())

	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	handler := &ToolHandler{store: store}

	session := db.NewSessionState("test-session")
	store.SaveSession(session)

	args := CompleteDesignArgs{
		SessionID:   "test-session",
		ArtifactPath: "/tmp/artifact.md",
	}

	req := &mcp.CallToolRequest{}
	res, _, err := handler.CompleteDesign(context.Background(), req, args)
	if err != nil {
		t.Fatalf("CompleteDesign error: %v", err)
	}
	if res.IsError {
		t.Errorf("Expected success, got error: %v", res.Content)
	}
}

func TestGetInternalLogsHandler(t *testing.T) {
	handler := &ToolHandler{}
	args := GetInternalLogsArgs{
		MaxLines: 10,
	}

	req := &mcp.CallToolRequest{}
	res, _, err := handler.GetInternalLogs(context.Background(), req, args)
	if err != nil {
		t.Fatalf("GetInternalLogs error: %v", err)
	}
	if res.IsError {
		t.Errorf("Expected success, got error")
	}
}

func TestUpdateConfigHandler(t *testing.T) {
	origDir := os.Getenv("XDG_CONFIG_HOME")
	tmpDir := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", origDir)

	configDir := filepath.Join(tmpDir, "mcp-server-magicdev")
	os.MkdirAll(configDir, 0700)
	configPath := filepath.Join(configDir, "magicdev.yaml")
	os.WriteFile(configPath, []byte(config.DefaultConfigTemplate), 0644)

	handler := &ToolHandler{}
	args := UpdateConfigArgs{
		Key:   "llm.model",
		Value: "gpt-4",
	}

	req := &mcp.CallToolRequest{}
	res, _, err := handler.UpdateConfig(context.Background(), req, args)
	if err != nil {
		t.Fatalf("UpdateConfig error: %v", err)
	}
	if res.IsError {
		msg := "unknown error"
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				msg = tc.Text
			}
		}
		t.Errorf("Expected success, got error: %s", msg)
	}
}

func TestClarifyRequirementsHandler(t *testing.T) {
	tmpDB, _ := os.CreateTemp("", "testdb-*.db")
	defer os.Remove(tmpDB.Name())
	viper.Set("server.db_path", tmpDB.Name())

	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	handler := &ToolHandler{store: store}

	session := db.NewSessionState("test-session-clarify")
	store.SaveSession(session)

	args := ClarifyRequirementsArgs{
		SessionID: "test-session-clarify",
		IsVetted:  false,
		SkepticAnalysis: &db.SkepticAnalysis{
			Narrative: "Skeptic",
		},
		SynthesisResolution: &db.SynthesisResolution{
			Decisions: []db.ArchitecturalDecision{{Topic: "t"}},
		},
	}

	req := &mcp.CallToolRequest{}
	res, _, err := handler.ClarifyRequirements(context.Background(), req, args)
	if err != nil {
		t.Fatalf("ClarifyRequirements error: %v", err)
	}
	if !res.IsError {
		t.Errorf("Expected error due to IsVetted=false, got success")
	}
}
