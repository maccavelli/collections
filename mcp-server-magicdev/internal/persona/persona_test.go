package persona

import (
	"strings"
	"testing"

	"mcp-server-magicdev/internal/db"
)

func TestGeneratePersona_NodeProject(t *testing.T) {
	session := &db.SessionState{
		TechStack:         "Node",
		TargetEnvironment: "local-ide",
		Labels:            []string{"domain:devtools", "domain:mcp-server"},
		DesignProposal: &db.DesignProposal{
			SecurityMandates: []db.SecurityItem{
				{Category: "input-validation", Description: "Validate paths", Severity: "HIGH", MitigationStrategy: "Resolve and check"},
			},
			StackTuning: []db.StackOptimization{
				{Category: "deps", Recommendation: "Prefer native Node.js APIs over third-party packages", Rationale: "Minimize attack surface", Priority: "must-have"},
			},
			ProposedModules: []db.ModuleSpec{
				{Name: "scanner", Purpose: "File traversal"},
				{Name: "security-checker", Purpose: "OWASP detection"},
			},
		},
	}

	persona := GeneratePersona(session, nil)

	checks := []struct {
		label   string
		content string
	}{
		{"tech expertise", "Node.js and TypeScript"},
		{"domain", "devtools"},
		{"environment", "local development tooling"},
		{"security", "input-validation"},
		{"priority", "prefer native Node.js APIs"},
		{"modules", "scanner"},
	}

	for _, c := range checks {
		if !strings.Contains(strings.ToLower(persona), strings.ToLower(c.content)) {
			t.Errorf("Missing %s: expected %q in persona:\n%s", c.label, c.content, persona)
		}
	}
}

func TestGeneratePersona_DotNet(t *testing.T) {
	session := &db.SessionState{
		TechStack:              ".NET",
		TargetEnvironment:      "containerized",
		ComplianceRequirements: []string{"SOC2", "HIPAA"},
	}

	persona := GeneratePersona(session, nil)

	if !strings.Contains(persona, ".NET platform") {
		t.Errorf("Expected .NET expertise, got: %s", persona)
	}
	if !strings.Contains(persona, "SOC2, HIPAA") {
		t.Errorf("Expected compliance requirements, got: %s", persona)
	}
	if !strings.Contains(persona, "containerized microservices") {
		t.Errorf("Expected container expertise, got: %s", persona)
	}
}

func TestGeneratePersona_NilSession(t *testing.T) {
	if GeneratePersona(nil, nil) != "" {
		t.Error("Expected empty string for nil session")
	}
}

func TestGeneratePersona_MinimalSession(t *testing.T) {
	session := &db.SessionState{
		TechStack: "Go",
	}

	persona := GeneratePersona(session, nil)

	if !strings.Contains(persona, "Go systems programming") {
		t.Errorf("Expected Go expertise, got: %s", persona)
	}
}

func TestGeneratePersona_UnknownStack(t *testing.T) {
	session := &db.SessionState{
		TechStack: "Rust",
	}

	persona := GeneratePersona(session, nil)

	if !strings.Contains(persona, "Rust development") {
		t.Errorf("Expected fallback expertise for unknown stack, got: %s", persona)
	}
}
