package handler

import (
	"strings"
	"testing"

	"mcp-server-magicdev/internal/db"
)

func TestBuildComprehensiveSpec(t *testing.T) {
	session := &db.SessionState{
		OriginalIdea:           "Test Idea",
		RefinedIdea:            "Refined Test Idea",
		TechStack:              "Go",
		TargetEnvironment:      "cloud",
		RiskLevel:              "Low",
		ComplianceRequirements: []string{"GDPR"},
		Labels:                 []string{"backend"},
		JiraID:                 "TEST-123",
		JiraBrowseURL:          "https://jira/TEST-123",
		FinalSpec:              "Golden Spec",
		SynthesisResolution: &db.SynthesisResolution{
			Decisions: []db.ArchitecturalDecision{
				{Topic: "DB", Decision: "Postgres", Rationale: "SQL"},
			},
		},
		DesignProposal: &db.DesignProposal{
			Narrative: "Proposal",
			ProposedModules: []db.ModuleSpec{
				{Name: "Core", Purpose: "Core logic", Responsibilities: []string{"Logic"}, Dependencies: []string{"DB"}},
			},
			TemplateAST: []db.FileEntry{
				{Path: "main.go", Type: "file", Language: "Go", Description: "entry", Exports: []string{"main"}},
			},
			StackTuning: []db.StackOptimization{
				{Category: "Perf", Recommendation: "Cache", Rationale: "Fast", Priority: "High"},
			},
			ReferencedStandards: []db.StandardReference{
				{StandardURL: "url", Rule: "rule", Application: "app"},
			},
			SecurityMandates: []db.SecurityItem{
				{Category: "Auth", Severity: "High", Description: "JWT", MitigationStrategy: "verify"},
			},
		},
		SkepticAnalysis: &db.SkepticAnalysis{
			Narrative: "Skeptic",
			Vulnerabilities: []db.SecurityItem{
				{Category: "XSS", Severity: "High", Description: "Escape", MitigationStrategy: "sanitize"},
			},
			DesignConcerns: []db.DesignConcern{
				{Area: "Scale", Severity: "Medium", Concern: "DB limit", Suggestion: "Pool"},
			},
		},
	}

	bp := &db.Blueprint{
		ImplementationStrategy: map[string]string{"req": "pat"},
		FileStructure: []db.FileEntry{
			{Path: "file", Type: "f", Language: "Go", Description: "d"},
		},
		DependencyManifest: []db.Dependency{
			{Name: "pkg", Version: "1", Ecosystem: "Go", Purpose: "p", License: "MIT", DevOnly: true},
		},
		SecurityConsiderations: []db.SecurityItem{
			{Category: "c", Severity: "s", Description: "d", MitigationStrategy: "m"},
		},
		NonFunctionalRequirements: []db.NFR{
			{Category: "c", Requirement: "r", Target: "t", Priority: "p"},
		},
		MCPTools: []db.MCPTool{
			{Name: "tool", Description: "d", InputSchema: "s"},
		},
		MCPResources: []db.MCPResource{
			{URI: "uri", Name: "n", Description: "d"},
		},
		MCPPrompts: []db.MCPPrompt{
			{Name: "p", Description: "d", Arguments: []string{"a"}},
		},
		ComplexityScores: map[string]int{"feat": 5},
		TestingStrategy:  map[string]string{"feat": "unit"},
		ADRs: []db.ADR{
			{Title: "adr1", Status: "acc", DecisionDate: "date", Context: "ctx", Decision: "dec", Consequences: "con", Alternatives: []db.Alternative{{Name: "alt", Pros: "p", Cons: "c", RejectionReason: "r"}}},
		},
		AporiaTraceability: map[string]string{"con": "res"},
		D2Source:           "x -> y",
	}

	meta := &db.SessionMetadata{
		ComplexityScore:   13,
		SecurityFootprint: "High",
		PatternPreference: "Clean",
		SocraticHistory:   "Log",
	}

	res := buildComprehensiveSpec(session, bp, meta, "Agent note", "Title")

	if !strings.Contains(res, "Test Idea") {
		t.Error("Missing OriginalIdea")
	}
	if !strings.Contains(res, "Postgres") {
		t.Error("Missing Decision")
	}
	if !strings.Contains(res, "Agent note") {
		t.Error("Missing Agent markdown")
	}
}

func TestBuildComprehensiveSpecNilBlueprint(t *testing.T) {
	session := &db.SessionState{}
	res := buildComprehensiveSpec(session, nil, nil, "Agent note", "Title")
	if !strings.Contains(res, "Agent note") {
		t.Error("Missing Agent markdown")
	}
}
