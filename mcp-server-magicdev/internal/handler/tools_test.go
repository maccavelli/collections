package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magicdev/internal/db"
)

func TestToolHandlers(t *testing.T) {
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	h := &ToolHandler{store: store}
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	// Test EvaluateIdea
	res, _, err := h.EvaluateIdea(ctx, req, EvaluateIdeaArgs{
		RawIdea:     "Test",
		TargetStack: ".NET",
	})
	if err != nil || res.IsError {
		t.Errorf("EvaluateIdea failed: %v, %v", err, res)
	}
	content := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "clarify_requirements") {
		t.Errorf("EvaluateIdea output missing handoff: %s", content)
	}
	// We won't try to parse the random session ID, we'll just create a known one for the rest of the tests.
	sessionID := "test-session-1"
	session := db.NewSessionState(sessionID)
	session.TechStack = ".NET"
	_ = store.SaveSession(session)

	// Test ClarifyRequirements
	res, _, err = h.ClarifyRequirements(ctx, req, ClarifyRequirementsArgs{
		SessionID:    sessionID,
		UserResponse: "Gap 1\nGap 2",
	})
	if err != nil || res.IsError {
		t.Errorf("ClarifyRequirements failed: %v", err)
	}
	content = res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "ingest_standards") {
		t.Errorf("ClarifyRequirements output missing handoff: %s", content)
	}

	// Test IngestStandards
	res, _, err = h.IngestStandards(ctx, req, IngestStandardsArgs{
		SessionID: sessionID,
		SourceURL: "Use Minimal APIs",
	})
	if err != nil || res.IsError {
		t.Errorf("IngestStandards failed: %v", err)
	}
	content = res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "critique_design") {
		t.Errorf("IngestStandards output missing handoff: %s", content)
	}

	// Test CritiqueDesign
	res, _, err = h.CritiqueDesign(ctx, req, CritiqueDesignArgs{
		SessionID:  sessionID,
		StrictMode: false,
	})
	if err != nil || res.IsError {
		t.Errorf("CritiqueDesign failed: %v", err)
	}
	content = res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "finalize_requirements") {
		t.Errorf("CritiqueDesign output missing handoff: %s", content)
	}

	// For FinalizeRequirements, store requires Tensions to be 0
	session, _ = store.LoadSession(sessionID)
	session.Tensions = []string{}
	_ = store.SaveSession(session)

	// Test FinalizeRequirements
	res, _, err = h.FinalizeRequirements(ctx, req, FinalizeRequirementsArgs{
		SessionID:         sessionID,
		ApprovalSignature: "Golden Spec Content",
	})
	if err != nil || res.IsError {
		t.Errorf("FinalizeRequirements failed: %v", err)
	}
	content = res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "blueprint_implementation") {
		t.Errorf("FinalizeRequirements output missing handoff: %s", content)
	}

	// Test BlueprintImplementation
	res, _, err = h.BlueprintImplementation(ctx, req, BlueprintImplementationArgs{
		SessionID:         sessionID,
		PatternPreference: "Clean Architecture",
	})
	if err != nil || res.IsError {
		t.Errorf("BlueprintImplementation failed: %v", err)
	}
	content = res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "generate_documents") {
		t.Errorf("Expected handoff response, got: %s", content)
	}

	// Test BlueprintImplementation error cases
	_, _, _ = h.BlueprintImplementation(ctx, req, BlueprintImplementationArgs{
		SessionID: "non-existent",
	})
}

func TestCompleteDesign(t *testing.T) {
	store, _ := db.InitStore()
	defer store.Close()
	h := &ToolHandler{store: store}

	// Just verify it doesn't panic
	h.CompleteDesign(context.Background(), nil, CompleteDesignArgs{SessionID: "123"})
}

func TestRegisterTools(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, &mcp.ServerOptions{})
	store, _ := db.InitStore()
	defer store.Close()
	RegisterTools(s, store)
}

func TestGenerateDocuments(t *testing.T) {
	store, _ := db.InitStore()
	defer store.Close()
	h := &ToolHandler{store: store}

	// We don't want to actually push to GitLab or create Jira tickets during unit tests.
	// ProcessDocumentGeneration performs live HTTP calls. We'll skip the actual tool call
	// if we don't have mock clients, or we expect it to fail.
	res, _, _ := h.GenerateDocuments(context.Background(), nil, GenerateDocumentsArgs{
		SessionID:    "123",
		Title:        "test",
		Markdown:     "test",
		TargetBranch: "main",
	})

	if res == nil {
		t.Error("Expected result, even if it's an error result due to missing config")
	}
}
