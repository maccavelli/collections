package handler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"github.com/spf13/viper"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magicdev/internal/db"
)

func TestToolHandlers(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	viper.Set("git.token", "test-token")
	viper.Set("jira.api_key", "test-token")
	viper.Set("confluence.api_key", "test-token")
	viper.Set("jira.url", "https://example.com")
	viper.Set("confluence.url", "https://example.com")
	viper.Set("git.server_url", "https://example.com")
	
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
		RawIdea:           "Test",
		TargetStack:       ".NET",
		TargetEnvironment: "cloud",
		Labels:            []string{"ecommerce"},
		BusinessCase:      "Test business case",
	})
	var content string
	if err != nil || res.IsError {
		// Just skip error if example.com is not reachable or mock fails
	} else {
		content = res.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(content, "ingest_standards") {
			t.Errorf("EvaluateIdea output missing handoff: %s", content)
		}
	}
	// We won't try to parse the random session ID, we'll just create a known one for the rest of the tests.
	sessionID := "test-session-1"
	session := db.NewSessionState(sessionID)
	session.TechStack = ".NET"
	_ = store.SaveSession(session)

	// Test ClarifyRequirements
	res, _, err = h.ClarifyRequirements(ctx, req, ClarifyRequirementsArgs{
		SessionID: sessionID,
		DesignProposal: &db.DesignProposal{
			Narrative: "A modular .NET service with API, business logic, and data access layers using Clean Architecture principles.",
			ProposedModules: []db.ModuleSpec{
				{Name: "APILayer", Purpose: "HTTP request handling", Responsibilities: []string{"Route registration", "Input validation", "Authentication middleware", "Response formatting"}, Dependencies: []string{"BusinessLogic"}},
				{Name: "BusinessLogic", Purpose: "Core domain logic", Responsibilities: []string{"Order processing", "Inventory management", "Payment orchestration"}, Dependencies: []string{"DataAccess"}},
				{Name: "DataAccess", Purpose: "Database persistence", Responsibilities: []string{"Entity mapping", "Query execution", "Connection pooling", "Migration management"}},
			},
			TemplateAST: []db.FileEntry{
				{Path: "src/API/Program.cs", Type: "source", Language: "C#", Description: "Application entry point"},
				{Path: "src/Business/OrderService.cs", Type: "source", Language: "C#", Description: "Order domain logic"},
				{Path: "src/Data/AppDbContext.cs", Type: "source", Language: "C#", Description: "Entity Framework context"},
			},
			SecurityMandates: []db.SecurityItem{
				{Category: "auth", Description: "JWT bearer token validation", Severity: "high", MitigationStrategy: "Use ASP.NET Core JWT middleware"},
				{Category: "injection", Description: "SQL injection prevention", Severity: "critical", MitigationStrategy: "Parameterized queries via EF Core"},
			},
			StackTuning: []db.StackOptimization{
				{Category: "concurrency", Recommendation: "Use async/await for all I/O", Rationale: "Prevents thread pool starvation", Priority: "must-have"},
				{Category: "startup", Recommendation: "Configure Kestrel with connection limits", Rationale: "Prevent resource exhaustion under load", Priority: "should-have"},
			},
		},
		SkepticAnalysis: &db.SkepticAnalysis{
			Narrative: "The design is sound but has scalability gaps in the data access layer and missing rate limiting.",
			Vulnerabilities: []db.SecurityItem{
				{Category: "dos", Description: "No rate limiting on API endpoints", Severity: "high", MitigationStrategy: "Add ASP.NET Core rate limiting middleware"},
			},
			DesignConcerns: []db.DesignConcern{
				{Area: "scalability", Concern: "Single database connection string", Severity: "medium", Suggestion: "Implement read replicas for query distribution"},
				{Area: "observability", Concern: "No structured logging or distributed tracing", Severity: "high", Suggestion: "Add OpenTelemetry with Serilog"},
			},
		},
		SynthesisResolution: &db.SynthesisResolution{
			Narrative: "Resolved conflicts between API simplicity and security requirements. Rate limiting added as mandatory.",
			Decisions: []db.ArchitecturalDecision{
				{Topic: "Rate Limiting", Decision: "Use ASP.NET Core rate limiter to mitigate dos attacks", Rationale: "Native middleware, zero external dependencies"},
				{Topic: "Observability", Decision: "Add OpenTelemetry with Serilog", Rationale: "Industry standard for distributed tracing"},
				{Topic: "Scalability", Decision: "Implement read replicas for query distribution", Rationale: "Prevents single database bottleneck"},
			},
		},
		IsVetted: true,
	})
	if err != nil || res.IsError {
		t.Errorf("ClarifyRequirements failed: %v", err)
	}
	content = res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "critique_design") {
		t.Errorf("ClarifyRequirements output missing handoff: %s", content)
	}

	// Test IngestStandards
	res, _, err = h.IngestStandards(ctx, req, IngestStandardsArgs{
		SessionID: sessionID,
		SourceURL: "https://example.com",
	})
	if err != nil || res.IsError {
		// Just skip error if example.com is not reachable in tests
	} else {
		content = res.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(content, "clarify_requirements") {
			t.Errorf("IngestStandards output missing handoff: %s", content)
		}
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

	// FinalizeRequirements can proceed directly after critique_design

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
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()
	h := &ToolHandler{store: store}

	// Just verify it doesn't panic
	h.CompleteDesign(context.Background(), nil, CompleteDesignArgs{SessionID: "123"})
}

func TestRegisterTools(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, &mcp.ServerOptions{})
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()
	RegisterTools(s, store)
}

func TestGenerateDocuments(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()
	h := &ToolHandler{store: store}

	// Create session so GenerateDocuments proceeds
	sessionID := "test-gen-docs"
	session := db.NewSessionState(sessionID)
	session.Blueprint = &db.Blueprint{}
	_ = store.SaveSession(session)

	// ProcessDocumentGeneration performs live HTTP calls, but we set mock flags in config.
	viper.Set("confluence.mock", true)
	viper.Set("jira.mock", true)
	res, _, _ := h.GenerateDocuments(context.Background(), nil, GenerateDocumentsArgs{
		SessionID:       sessionID,
		Title:           "test",
		Markdown:        "test",
		TargetBranch:    "main",
		DiagramOverride: "A -> B",
	})

	if res == nil {
		t.Error("Expected result, even if it's an error result")
	}
}

func TestEvaluateIdea_ValidationGate(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	h := &ToolHandler{store: store}
	ctx := context.Background()

	// Missing TargetEnvironment
	res1, _, _ := h.EvaluateIdea(ctx, nil, EvaluateIdeaArgs{
		RawIdea:      "Test",
		TargetStack:  ".NET",
		Labels:       []string{"erp"},
		BusinessCase: "Test",
	})
	content1 := res1.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content1, "[VALIDATION REQUIRED]") {
		t.Errorf("Expected validation error, got: %s", content1)
	}

	// Missing Labels
	res2, _, _ := h.EvaluateIdea(ctx, nil, EvaluateIdeaArgs{
		RawIdea:           "Test",
		TargetStack:       ".NET",
		TargetEnvironment: "cloud",
		BusinessCase:      "Test",
	})
	content2 := res2.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content2, "[VALIDATION REQUIRED]") {
		t.Errorf("Expected validation error, got: %s", content2)
	}

	// Missing Business Case
	res3, _, _ := h.EvaluateIdea(ctx, nil, EvaluateIdeaArgs{
		RawIdea:           "Test",
		TargetStack:       ".NET",
		TargetEnvironment: "cloud",
		Labels:            []string{"erp"},
	})
	content3 := res3.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content3, "[VALIDATION REQUIRED]") {
		t.Errorf("Expected validation error for business case, got: %s", content3)
	}
}

func TestDesignQualityGate(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	h := &ToolHandler{store: store}
	ctx := context.Background()

	sessionID := "test-quality-gate"
	session := db.NewSessionState(sessionID)
	session.TechStack = "Node"
	_ = store.SaveSession(session)

	// Thin data should be rejected by the quality gate
	res, _, _ := h.ClarifyRequirements(ctx, nil, ClarifyRequirementsArgs{
		SessionID: sessionID,
		DesignProposal: &db.DesignProposal{
			Narrative:       "thin",
			ProposedModules: []db.ModuleSpec{{Name: "core", Purpose: "main", Responsibilities: []string{"do stuff"}}},
			TemplateAST:     []db.FileEntry{{Path: "src/index.ts", Type: "file"}},
		},
		SkepticAnalysis: &db.SkepticAnalysis{Narrative: "thin"},
		SynthesisResolution: &db.SynthesisResolution{
			Narrative: "thin",
			Decisions: []db.ArchitecturalDecision{{Topic: "x", Decision: "y", Rationale: "z"}},
		},
		IsVetted: true,
	})
	content := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "DESIGN QUALITY GATE FAILED") {
		t.Errorf("Expected quality gate rejection, got: %s", content)
	}
	if !strings.Contains(content, "proposed_modules has 1 modules") {
		t.Error("Expected specific deficiency about module count")
	}
	if !strings.Contains(content, "security_mandates has 0 items") {
		t.Error("Expected specific deficiency about security mandates")
	}
}

func TestGoldenPathSynthesis(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	session := db.NewSessionState("test-golden-path")
	session.TechStack = "Node"
	session.Standards = []string{"Use TypeScript for all Node.js projects. Prefer ESM imports."}

	thesis := &db.DesignProposal{
		Narrative: "A modular Node.js service",
		ProposedModules: []db.ModuleSpec{
			{Name: "API", Purpose: "HTTP handling", Responsibilities: []string{"routing", "validation"}, Dependencies: []string{"Core"}},
			{Name: "Core", Purpose: "Business logic", Responsibilities: []string{"processing"}},
		},
		SecurityMandates: []db.SecurityItem{
			{Category: "auth", Description: "JWT validation", Severity: "high", MitigationStrategy: "Use middleware"},
		},
		StackTuning: []db.StackOptimization{
			{Category: "performance", Recommendation: "Use async/await", Rationale: "Prevents blocking", Priority: "must-have"},
		},
	}

	antithesis := &db.SkepticAnalysis{
		Narrative: "Rate limiting is missing",
		Vulnerabilities: []db.SecurityItem{
			{Category: "dos", Description: "No rate limiting", Severity: "high", MitigationStrategy: "Add rate limiter"},
		},
		DesignConcerns: []db.DesignConcern{
			{Area: "observability", Concern: "No logging", Severity: "medium", Suggestion: "Add structured logging"},
		},
	}

	synthesis := goldenPathSynthesis(store, session, thesis, antithesis, nil)

	if synthesis == nil {
		t.Fatal("Golden Path returned nil synthesis")
	}
	if !strings.Contains(synthesis.Narrative, "Golden Path Synthesis") {
		t.Errorf("Expected Golden Path narrative, got: %s", synthesis.Narrative)
	}
	if len(synthesis.Decisions) < 4 {
		t.Errorf("Expected at least 4 decisions (2 modules + 1 security + 1 tuning), got %d", len(synthesis.Decisions))
	}
	if len(synthesis.OutstandingQuestions) < 2 {
		t.Errorf("Expected at least 2 outstanding questions from skeptic, got %d", len(synthesis.OutstandingQuestions))
	}

	// Verify module decisions exist
	found := false
	for _, d := range synthesis.Decisions {
		if strings.Contains(d.Topic, "Module: API") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected module-level decision for 'API'")
	}
}

func TestConstraintLinterKeywords(t *testing.T) {
	session := &db.SessionState{
		OriginalIdea: "Build a database query service with file path resolution and authentication token handling",
		DesignProposal: &db.DesignProposal{
			Narrative: "Service that executes database queries and manages file uploads",
			ProposedModules: []db.ModuleSpec{
				{Name: "QueryEngine", Purpose: "Execute database queries", Responsibilities: []string{"query execution", "result mapping"}},
				{Name: "FileManager", Purpose: "Handle file uploads via path resolution", Responsibilities: []string{"path validation", "upload processing"}},
			},
		},
	}

	bp := &db.Blueprint{}
	enrichSecurityFromKeywords(bp, session)

	if len(bp.SecurityConsiderations) == 0 {
		t.Fatal("Expected keyword linter to inject security items, got 0")
	}

	// Check for injection rule (database + query)
	foundInjection := false
	foundPathTraversal := false
	foundAuth := false
	for _, item := range bp.SecurityConsiderations {
		if item.Category == "injection" {
			foundInjection = true
		}
		if item.Category == "path-traversal" {
			foundPathTraversal = true
		}
		if item.Category == "broken-auth" {
			foundAuth = true
		}
	}

	if !foundInjection {
		t.Error("Expected injection rule to fire (keywords: database + query)")
	}
	if !foundPathTraversal {
		t.Error("Expected path-traversal rule to fire (keywords: file + path)")
	}
	if !foundAuth {
		t.Error("Expected broken-auth rule to fire (keywords: auth + token)")
	}
}

func TestEnvironmentSpecificNFRs(t *testing.T) {
	// Test containerized environment
	sessionContainer := &db.SessionState{TargetEnvironment: "containerized"}
	bpContainer := &db.Blueprint{}
	enrichEnvironmentConstraints(bpContainer, sessionContainer)

	if len(bpContainer.NonFunctionalRequirements) == 0 {
		t.Fatal("Expected container NFRs to be injected")
	}

	foundHealth := false
	foundStateless := false
	for _, nfr := range bpContainer.NonFunctionalRequirements {
		if strings.Contains(nfr.Requirement, "Health check") {
			foundHealth = true
		}
		if strings.Contains(nfr.Requirement, "Stateless") {
			foundStateless = true
		}
	}
	if !foundHealth {
		t.Error("Expected health check NFR for containerized environment")
	}
	if !foundStateless {
		t.Error("Expected stateless design NFR for containerized environment")
	}

	// Verify container security mandate
	if len(bpContainer.SecurityConsiderations) == 0 {
		t.Error("Expected container security mandate (non-root execution)")
	}

	// Test local IDE environment — should NOT have container NFRs
	sessionLocal := &db.SessionState{TargetEnvironment: "local-ide"}
	bpLocal := &db.Blueprint{}
	enrichEnvironmentConstraints(bpLocal, sessionLocal)

	if len(bpLocal.NonFunctionalRequirements) == 0 {
		t.Fatal("Expected local-ide NFRs to be injected")
	}

	foundStdio := false
	for _, nfr := range bpLocal.NonFunctionalRequirements {
		if strings.Contains(nfr.Requirement, "stdio") {
			foundStdio = true
		}
	}
	if !foundStdio {
		t.Error("Expected stdio transport safety NFR for local-ide environment")
	}
}

func TestResolveQuestionsFromStandards(t *testing.T) {
	standards := []string{
		"Use @modelcontextprotocol/sdk for MCP server development. Always use globby for file discovery.",
		"TypeScript is the recommended language for Node.js projects.",
	}

	questions := []db.GranularQuestion{
		{Topic: "Standard Deviation: MCP Transport → @modelcontextprotocol/sdk", Question: "Should we deviate?", Context: "Not in standards", Impact: "high"},
		{Topic: "Standard Deviation: Audit Engine → globby", Question: "Should we deviate?", Context: "Not in standards", Impact: "medium"},
		{Topic: "Standard Deviation: Stack Tuning [Worker Threads]", Question: "Intentional deviation?", Context: "Not in standards", Impact: "medium"},
		{Topic: "Skeptic Vulnerability [Code Injection]", Question: "How to mitigate?", Context: "External tools", Impact: "high"},
	}

	synthesis := &db.SynthesisResolution{}
	remaining := resolveQuestionsFromStandards(questions, standards, synthesis)

	// @modelcontextprotocol/sdk and globby are in the standards, so those 2 should be resolved
	if len(remaining) != 2 {
		t.Errorf("Expected 2 remaining questions, got %d", len(remaining))
		for _, q := range remaining {
			t.Logf("  Remaining: %s", q.Topic)
		}
	}

	// Verify decisions were created for auto-resolved questions
	if len(synthesis.Decisions) != 2 {
		t.Errorf("Expected 2 auto-resolution decisions, got %d", len(synthesis.Decisions))
	}

	// Worker Threads and Code Injection should remain (Worker Threads isn't in standards, Code Injection isn't a deviation)
	for _, q := range remaining {
		if strings.Contains(q.Topic, "@modelcontextprotocol/sdk") || strings.Contains(q.Topic, "globby") {
			t.Errorf("Question should have been auto-resolved: %s", q.Topic)
		}
	}
}

func TestSocraticRetryEnforcement(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	h := &ToolHandler{store: store}
	ctx := context.Background()

	sessionID := "test-retry-enforcement"
	session := db.NewSessionState(sessionID)
	session.TechStack = "Node"
	session.StepStatus["evaluate_idea"] = "COMPLETED"
	session.StepStatus["ingest_standards"] = "COMPLETED"
	_ = store.SaveSession(session)

	// Build args that will always generate an unresolvable question
	// (skeptic vulnerability without a matching decision)
	args := ClarifyRequirementsArgs{
		SessionID: sessionID,
		DesignProposal: &db.DesignProposal{
			Narrative: "A modular service with API, business logic, and data layers",
			ProposedModules: []db.ModuleSpec{
				{Name: "API", Purpose: "HTTP", Responsibilities: []string{"routing", "validation", "middleware"}, Dependencies: []string{"Core"}},
				{Name: "Core", Purpose: "Logic", Responsibilities: []string{"processing", "orchestration", "events"}},
				{Name: "Data", Purpose: "Persistence", Responsibilities: []string{"queries", "migrations", "caching"}},
			},
			TemplateAST: []db.FileEntry{
				{Path: "src/index.ts", Type: "file"},
				{Path: "src/api/routes.ts", Type: "file"},
				{Path: "src/core/service.ts", Type: "file"},
			},
			SecurityMandates: []db.SecurityItem{
				{Category: "auth", Description: "JWT validation", Severity: "high", MitigationStrategy: "middleware"},
				{Category: "injection", Description: "SQL injection prevention", Severity: "critical", MitigationStrategy: "parameterized queries"},
			},
			StackTuning: []db.StackOptimization{
				{Category: "perf", Recommendation: "Async/await", Rationale: "Non-blocking", Priority: "must-have"},
				{Category: "memory", Recommendation: "Stream processing", Rationale: "Low memory", Priority: "should-have"},
			},
		},
		SkepticAnalysis: &db.SkepticAnalysis{
			Narrative: "Concerns exist",
			Vulnerabilities: []db.SecurityItem{
				{Category: "extremely-specific-unresolvable-issue", Description: "Cannot auto-resolve", Severity: "critical", MitigationStrategy: "Manual review"},
			},
			DesignConcerns: []db.DesignConcern{
				{Area: "very-specific-concern-alpha", Concern: "Issue A", Severity: "high", Suggestion: "Fix A"},
				{Area: "very-specific-concern-beta", Concern: "Issue B", Severity: "medium", Suggestion: "Fix B"},
			},
		},
		SynthesisResolution: &db.SynthesisResolution{
			Narrative: "Partial resolution",
			Decisions: []db.ArchitecturalDecision{
				{Topic: "general-1", Decision: "Decision 1", Rationale: "Reason 1"},
				{Topic: "general-2", Decision: "Decision 2", Rationale: "Reason 2"},
			},
		},
		IsVetted: true,
	}

	// Call 6 times — should get SOCRATIC CONFLICT on 1-5, then SESSION LOCKED on 6th
	for i := 1; i <= 6; i++ {
		res, _, _ := h.ClarifyRequirements(ctx, nil, args)
		content := res.Content[0].(*mcp.TextContent).Text

		if i <= 5 {
			if !strings.Contains(content, "SOCRATIC CONFLICT DETECTED") {
				t.Errorf("Attempt %d: Expected SOCRATIC CONFLICT, got: %s", i, content[:100])
			}
			if !strings.Contains(content, fmt.Sprintf("attempt %d of 5", i)) {
				t.Errorf("Attempt %d: Expected attempt counter in message", i)
			}
		} else {
			if !strings.Contains(content, "SOCRATIC SESSION LOCKED") {
				t.Errorf("Attempt %d: Expected SESSION LOCKED, got: %s", i, content[:100])
			}
		}
	}

	// Verify retry counter was incremented
	session, _ = store.LoadSession(sessionID)
	if session.SocraticRetryCount < 5 {
		t.Errorf("Expected retry count >= 5, got %d", session.SocraticRetryCount)
	}
}

func TestTrifectaDataProtection(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	h := &ToolHandler{store: store}
	ctx := context.Background()

	sessionID := "test-trifecta-protection"
	session := db.NewSessionState(sessionID)
	session.TechStack = "Node"
	session.StepStatus["evaluate_idea"] = "COMPLETED"
	session.StepStatus["ingest_standards"] = "COMPLETED"
	_ = store.SaveSession(session)

	// First call with rich skeptic data
	originalSkeptic := &db.SkepticAnalysis{
		Narrative: "Rich analysis with concerns",
		Vulnerabilities: []db.SecurityItem{
			{Category: "unresolvable-vuln", Description: "Critical issue", Severity: "critical", MitigationStrategy: "Fix it"},
		},
		DesignConcerns: []db.DesignConcern{
			{Area: "unresolvable-concern-1", Concern: "Missing caching", Severity: "high", Suggestion: "Add Redis"},
			{Area: "unresolvable-concern-2", Concern: "No monitoring", Severity: "medium", Suggestion: "Add Prometheus"},
		},
	}

	args := ClarifyRequirementsArgs{
		SessionID: sessionID,
		DesignProposal: &db.DesignProposal{
			Narrative: "A modular service with API, business logic, and data layers",
			ProposedModules: []db.ModuleSpec{
				{Name: "API", Purpose: "HTTP", Responsibilities: []string{"routing", "validation", "middleware"}, Dependencies: []string{"Core"}},
				{Name: "Core", Purpose: "Logic", Responsibilities: []string{"processing", "orchestration", "events"}},
				{Name: "Data", Purpose: "Persistence", Responsibilities: []string{"queries", "migrations", "caching"}},
			},
			TemplateAST:      []db.FileEntry{{Path: "src/index.ts", Type: "file"}, {Path: "src/api/routes.ts", Type: "file"}, {Path: "src/core/service.ts", Type: "file"}},
			SecurityMandates: []db.SecurityItem{{Category: "auth", Description: "JWT", Severity: "high", MitigationStrategy: "middleware"}, {Category: "injection", Description: "SQLi", Severity: "critical", MitigationStrategy: "parameterized"}},
			StackTuning:      []db.StackOptimization{{Category: "perf", Recommendation: "Async", Rationale: "Non-blocking", Priority: "must-have"}, {Category: "memory", Recommendation: "Streams", Rationale: "Low mem", Priority: "should-have"}},
		},
		SkepticAnalysis:     originalSkeptic,
		SynthesisResolution: &db.SynthesisResolution{Narrative: "Partial", Decisions: []db.ArchitecturalDecision{{Topic: "gen-1", Decision: "D1", Rationale: "R1"}, {Topic: "gen-2", Decision: "D2", Rationale: "R2"}}},
		IsVetted:            true,
	}

	// First call — will trigger SOCRATIC CONFLICT, but should store the data
	h.ClarifyRequirements(ctx, nil, args)

	// Verify original data was stored
	session, _ = store.LoadSession(sessionID)
	if session.SkepticAnalysis == nil {
		t.Fatal("SkepticAnalysis should be stored after first call")
	}
	if len(session.SkepticAnalysis.Vulnerabilities) != 1 {
		t.Errorf("Expected 1 vulnerability stored, got %d", len(session.SkepticAnalysis.Vulnerabilities))
	}

	// Second call with EMPTY skeptic data (simulating agent stripping data on re-entry)
	args.SkepticAnalysis = &db.SkepticAnalysis{
		Narrative:       "Stripped",
		Vulnerabilities: []db.SecurityItem{},
		DesignConcerns:  []db.DesignConcern{},
	}
	h.ClarifyRequirements(ctx, nil, args)

	// Verify original data was NOT overwritten
	session, _ = store.LoadSession(sessionID)
	if session.SkepticAnalysis == nil {
		t.Fatal("SkepticAnalysis should still be present")
	}
	if len(session.SkepticAnalysis.Vulnerabilities) != 1 {
		t.Errorf("Expected 1 vulnerability preserved (not overwritten), got %d", len(session.SkepticAnalysis.Vulnerabilities))
	}
	if len(session.SkepticAnalysis.DesignConcerns) != 2 {
		t.Errorf("Expected 2 design concerns preserved, got %d", len(session.SkepticAnalysis.DesignConcerns))
	}
}

func TestRetryCounterResetOnSuccess(t *testing.T) {
	viper.Set("server.db_path", ":memory:")
	store, err := db.InitStore()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer store.Close()

	h := &ToolHandler{store: store}
	ctx := context.Background()

	sessionID := "test-retry-reset"
	session := db.NewSessionState(sessionID)
	session.TechStack = ".NET"
	session.StepStatus["evaluate_idea"] = "COMPLETED"
	session.StepStatus["ingest_standards"] = "COMPLETED"
	session.SocraticRetryCount = 2
	session.SocraticEscalatedTopics = []string{"old-topic"}
	_ = store.SaveSession(session)

	// Call with fully resolved data — all skeptic concerns addressed by decisions
	res, _, _ := h.ClarifyRequirements(ctx, nil, ClarifyRequirementsArgs{
		SessionID: sessionID,
		DesignProposal: &db.DesignProposal{
			Narrative: "A modular .NET service with API, business logic, and data layers",
			ProposedModules: []db.ModuleSpec{
				{Name: "API", Purpose: "HTTP", Responsibilities: []string{"routing", "validation", "middleware"}, Dependencies: []string{"Core"}},
				{Name: "Core", Purpose: "Logic", Responsibilities: []string{"processing", "orchestration", "events"}},
				{Name: "Data", Purpose: "Persistence", Responsibilities: []string{"queries", "migrations", "caching"}},
			},
			TemplateAST:      []db.FileEntry{{Path: "src/Program.cs", Type: "file"}, {Path: "src/API/Controller.cs", Type: "file"}, {Path: "src/Data/Context.cs", Type: "file"}},
			SecurityMandates: []db.SecurityItem{{Category: "auth", Description: "JWT", Severity: "high", MitigationStrategy: "middleware"}, {Category: "injection", Description: "SQLi", Severity: "critical", MitigationStrategy: "EF Core"}},
			StackTuning:      []db.StackOptimization{{Category: "perf", Recommendation: "Async", Rationale: "Non-blocking", Priority: "must-have"}, {Category: "startup", Recommendation: "Kestrel config", Rationale: "Throughput", Priority: "should-have"}},
		},
		SkepticAnalysis: &db.SkepticAnalysis{
			Narrative:       "Concerns addressed",
			Vulnerabilities: []db.SecurityItem{{Category: "dos", Description: "No rate limiting", Severity: "high", MitigationStrategy: "middleware"}},
			DesignConcerns:  []db.DesignConcern{{Area: "observability", Concern: "No logging", Severity: "medium", Suggestion: "Add Serilog"}, {Area: "scalability", Concern: "Single DB", Severity: "medium", Suggestion: "Add replicas"}},
		},
		SynthesisResolution: &db.SynthesisResolution{
			Narrative: "Fully resolved",
			Decisions: []db.ArchitecturalDecision{
				{Topic: "dos", Decision: "Add rate limiting", Rationale: "Security"},
				{Topic: "observability", Decision: "Add Serilog", Rationale: "Monitoring"},
				{Topic: "scalability", Decision: "Add replicas", Rationale: "Performance"},
			},
		},
		IsVetted: true,
	})

	content := res.Content[0].(*mcp.TextContent).Text
	if strings.Contains(content, "SOCRATIC CONFLICT") {
		t.Errorf("Expected successful resolution, got conflict: %s", content[:200])
	}

	// Verify retry counter was reset
	session, _ = store.LoadSession(sessionID)
	if session.SocraticRetryCount != 0 {
		t.Errorf("Expected retry count to be reset to 0, got %d", session.SocraticRetryCount)
	}
	if len(session.SocraticEscalatedTopics) != 0 {
		t.Errorf("Expected escalated topics to be cleared, got %d", len(session.SocraticEscalatedTopics))
	}
}
