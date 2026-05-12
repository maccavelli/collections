package integration

import (
	"strings"
	"testing"

	"mcp-server-magicdev/internal/db"
)

func TestBuildMADRDocument(t *testing.T) {
	session := &db.SessionState{
		TechStack:              "Go",
		TargetEnvironment:      "Docker",
		ComplianceRequirements: []string{"SOC2"},
		Labels:                 []string{"backend"},
		DesignProposal: &db.DesignProposal{
			Narrative: "Proposal",
			ProposedModules: []db.ModuleSpec{
				{Name: "Core", Purpose: "Logic", Responsibilities: []string{"Logic"}, Dependencies: []string{"DB"}},
			},
			SecurityMandates: []db.SecurityItem{
				{Category: "Auth", Severity: "High", Description: "JWT", MitigationStrategy: "verify"},
			},
			TemplateAST: []db.FileEntry{
				{Path: "main.go", Type: "file", Language: "Go", Description: "entry"},
			},
		},
	}

	bp := &db.Blueprint{
		ADRs: []db.ADR{
			{
				Title: "adr1", Status: "accepted", Context: "ctx", Decision: "dec", Consequences: "con", 
				DecisionDate: "2024-01-01", DecisionDrivers: []string{"driver1"}, Tags: []string{"tag1"}, 
				ComplianceCheck: "soc2", Confirmation: "confirm", Supersedes: "adr0", SecurityFootprint: "low",
				Alternatives: []db.Alternative{{Name: "alt", Pros: "p", Cons: "c", RejectionReason: "r"}},
			},
		},
		TestingStrategy: map[string]string{"Auth": "Unit"},
		ImplementationStrategy: map[string]string{"Feature1": "Strategy1"},
		NonFunctionalRequirements: []db.NFR{
			{Category: "Performance", Target: "100ms", Requirement: "Fast", Priority: "High"},
		},
		MCPTools: []db.MCPTool{
			{Name: "tool1", Description: "desc", InputSchema: "{}"},
		},
		MCPPrompts: []db.MCPPrompt{
			{Name: "prompt1", Description: "desc", Arguments: []string{"arg1"}},
		},
		MCPResources: []db.MCPResource{
			{URI: "file:///test", Name: "res1", Description: "desc"},
		},
	}
	session.SynthesisResolution = &db.SynthesisResolution{
		ConstraintLocks: []db.ChaosConstraint{
			{Domain: "Core", Platform: "Go", Constraint: "const", Impact: "high", Enforced: true},
		},
	}
	session.ChaosAnalysis = &db.ChaosAnalysis{
		RejectedPatterns: []db.ChaosRejection{
			{Pattern: "pat1", Reason: "bad", Severity: "High", Source: "Graveyard"},
		},
		StressScenarios: []db.StressScenario{
			{Scenario: "load", Trigger: "reqs", Impact: "fail", Mitigation: "scale"},
		},
	}

	madr := buildMADRDocument("Test Project", session, bp, "Test Markdown", "Test Browse")

	if !strings.Contains(madr, "Test Project") {
		t.Error("Missing Title")
	}
	if !strings.Contains(madr, "Auth") {
		t.Error("Missing Auth security mandate")
	}
}

func TestBuildMADRDocumentNilPointers(t *testing.T) {
	session := &db.SessionState{}
	bp := &db.Blueprint{}
	madr := buildMADRDocument("Test Project", session, bp, "", "")
	if !strings.Contains(madr, "Test Project") {
		t.Error("Missing Title")
	}
}
