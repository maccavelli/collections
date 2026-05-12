package handler

import (
	"strings"
	"testing"

	"mcp-server-magicdev/internal/db"
)

func TestValidateDesignQuality(t *testing.T) {
	err := validateDesignQuality(nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "design_proposal is required") {
		t.Errorf("Expected design_proposal missing error, got %v", err)
	}

	proposal := &db.DesignProposal{
		Narrative: "test",
		ProposedModules: []db.ModuleSpec{
			{Name: "1", Responsibilities: []string{"a", "b", "c"}},
			{Name: "2", Responsibilities: []string{"a", "b", "c"}},
			{Name: "3", Responsibilities: []string{"a", "b", "c"}},
		},
		TemplateAST: []db.FileEntry{{}, {}, {}},
		SecurityMandates: []db.SecurityItem{{}, {}},
		StackTuning: []db.StackOptimization{{}, {}},
	}
	skeptic := &db.SkepticAnalysis{
		Narrative: "test",
		Vulnerabilities: []db.SecurityItem{{}},
		DesignConcerns: []db.DesignConcern{{}, {}},
	}
	synthesis := &db.SynthesisResolution{
		Decisions: []db.ArchitecturalDecision{{}, {}},
	}

	err = validateDesignQuality(proposal, skeptic, synthesis)
	if err != nil {
		t.Errorf("Expected valid proposal, got %v", err)
	}
}

func TestEnrichBlueprintFromSession(t *testing.T) {
	bp := &db.Blueprint{}
	session := &db.SessionState{
		TechStack: "golang",
		DesignProposal: &db.DesignProposal{
			ProposedModules: []db.ModuleSpec{
				{Name: "api", Purpose: "api"},
				{Name: "db", Purpose: "db"},
			},
		},
	}

	enrichBlueprintFromSession(bp, session)

	if len(bp.FileStructure) < 2 {
		t.Errorf("Expected enriched file structure")
	}
	if len(bp.NonFunctionalRequirements) == 0 {
		t.Errorf("Expected enriched NFRs")
	}
	if len(bp.TestingStrategy) == 0 {
		t.Errorf("Expected enriched testing strategy")
	}
	if len(bp.ComplexityScores) == 0 {
		t.Errorf("Expected enriched complexity scores")
	}
}
