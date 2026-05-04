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

	sessionID := "test-session-1"

	// Test EvaluateIdea
	res, _, err := h.EvaluateIdea(ctx, req, EvaluateIdeaArgs{
		SessionID: sessionID,
		TechStack: ".NET",
	})
	if err != nil || res.IsError {
		t.Errorf("EvaluateIdea failed: %v, %v", err, res)
	}

	// Test ClarifyRequirements
	res, _, err = h.ClarifyRequirements(ctx, req, ClarifyRequirementsArgs{
		SessionID: sessionID,
		Findings:  "Gap 1\nGap 2",
	})
	if err != nil || res.IsError {
		t.Errorf("ClarifyRequirements failed: %v", err)
	}

	// Test IngestStandard
	res, _, err = h.IngestStandard(ctx, req, IngestStandardArgs{
		SessionID: sessionID,
		Standard:  "Use Minimal APIs",
	})
	if err != nil || res.IsError {
		t.Errorf("IngestStandard failed: %v", err)
	}

	// Test CritiqueDesign
	res, _, err = h.CritiqueDesign(ctx, req, CritiqueDesignArgs{
		SessionID: sessionID,
		Design:    "Test Design",
	})
	if err != nil || res.IsError {
		t.Errorf("CritiqueDesign failed: %v", err)
	}
	content := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "Use Minimal APIs") {
		t.Errorf("CritiqueDesign output missing standard: %s", content)
	}

	// Test FinalizeRequirements
	res, _, err = h.FinalizeRequirements(ctx, req, FinalizeRequirementsArgs{
		SessionID:  sessionID,
		GoldenSpec: "Golden Spec Content",
	})
	if err != nil || res.IsError {
		t.Errorf("FinalizeRequirements failed: %v", err)
	}

	// Test BlueprintImplementation (Fallback path since req.Session == nil)
	res, _, err = h.BlueprintImplementation(ctx, req, BlueprintImplementationArgs{
		SessionID: sessionID,
	})
	if err != nil || res.IsError {
		t.Errorf("BlueprintImplementation failed: %v", err)
	}
	content = res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "Sampling unavailable") {
		t.Errorf("Expected fallback response, got: %s", content)
	}

	// Test BlueprintImplementation error cases
	_, _, _ = h.BlueprintImplementation(ctx, req, BlueprintImplementationArgs{
		SessionID: "non-existent",
	})
}

func TestBuildBlueprintSummary(t *testing.T) {
	bp := &db.Blueprint{
		ImplementationStrategy: map[string]string{"req1": "pattern1"},
		DependencyManifest: []db.Dependency{
			{Name: "pkg", Version: "1.0", Ecosystem: "nuget"},
		},
		ComplexityScores: map[string]int{"feature1": 5},
		AporiaTraceability: map[string]string{"gap": "fix"},
	}

	summary := buildBlueprintSummary(bp, ".NET")
	if !strings.Contains(summary, "pattern1") || !strings.Contains(summary, "pkg@1.0") || !strings.Contains(summary, "5 SP") {
		t.Errorf("Summary missing expected content: %s", summary)
	}
}

func TestBuildBlueprintSamplingPrompt(t *testing.T) {
	session := db.NewSessionState("test")
	session.TechStack = ".NET"
	session.FinalSpec = "Spec"
	session.Standards = []string{"Std"}
	session.AporiaResolutions = []string{"Aporia"}

	prompt := buildBlueprintSamplingPrompt(session)
	if !strings.Contains(prompt, "Spec") || !strings.Contains(prompt, "Std") || !strings.Contains(prompt, "Aporia") || !strings.Contains(prompt, ".NET 9+") {
		t.Errorf("Prompt missing expected content: %s", prompt)
	}
}

func TestAttemptSampling(t *testing.T) {
	_, err := attemptSampling(context.Background(), nil, "prompt", ".NET")
	if err == nil {
		t.Error("Expected error when req is nil")
	}
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
	
	res, _, _ := h.GenerateDocuments(context.Background(), nil, GenerateDocumentsArgs{
		SessionID: "123",
		Title: "test",
		Markdown: "test",
		RepoPath: "/test",
	})
	
	if res == nil {
		t.Error("Expected result")
	}
}
