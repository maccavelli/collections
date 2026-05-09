package integration

import (
	"strings"
	"testing"

	"mcp-server-magicdev/internal/db"
)

func TestBuildMADRDocument(t *testing.T) {
	session := &db.SessionState{
		SessionID:         "session-test-123",
		TechStack:         "Node",
		TargetEnvironment: "local-ide",
		BusinessCase:      "Audit Node.js codebases for best practices",
		RefinedIdea:       "A TypeScript MCP server for codebase auditing",
		Labels:            []string{"domain:devtools", "type:mcp-server"},
		RiskLevel:         "medium",
		DesignProposal: &db.DesignProposal{
			Narrative: "The system uses a modular tool pipeline.",
			ProposedModules: []db.ModuleSpec{
				{Name: "scanner", Purpose: "File traversal", Responsibilities: []string{"walk", "classify"}, Dependencies: []string{"security-checker"}},
			},
			SecurityMandates: []db.SecurityItem{
				{Category: "input-validation", Description: "Validate paths", Severity: "HIGH", MitigationStrategy: "Resolve and check"},
			},
			StackTuning: []db.StackOptimization{
				{Category: "deps", Recommendation: "Prefer native APIs", Rationale: "Less attack surface", Priority: "must-have"},
			},
		},
		SkepticAnalysis: &db.SkepticAnalysis{
			Vulnerabilities: []db.SecurityItem{
				{Category: "path-traversal", Description: "Symlink attack", Severity: "HIGH", MitigationStrategy: "Use realpath"},
			},
			DesignConcerns: []db.DesignConcern{
				{Area: "architecture", Concern: "over-abstract the scanner", Severity: "medium", Suggestion: "Keep it simple"},
			},
		},
		SynthesisResolution: &db.SynthesisResolution{
			Decisions: []db.ArchitecturalDecision{
				{Topic: "Transport", Decision: "Use stdio", Rationale: "IDE compatibility"},
			},
		},
	}

	bp := &db.Blueprint{
		ADRs: []db.ADR{
			{
				Title:           "ESLint Strategy",
				Status:          "Accepted",
				Context:         "ESLint is heavy but useful",
				Decision:        "Optional peer dependency",
				Consequences:    "Projects without ESLint get AST-only analysis",
				DecisionDate:    "2026-05-08",
				DecisionDrivers: []string{"Minimize attack surface", "Support existing configs"},
				Tags:            []string{"architecture", "dependencies"},
				Alternatives: []db.Alternative{
					{Name: "Full ESLint as direct dep", Pros: "Maximum coverage", Cons: "Heavy transitive deps", RejectionReason: "Increases attack surface"},
				},
				Confirmation: "Verify ESLint detected via require.resolve",
			},
		},
		FileStructure: []db.FileEntry{
			{Path: "src/index.ts", Type: "source", Language: "TypeScript", Description: "Entry point", Exports: []string{"main"}},
		},
		DataModel: []db.Entity{
			{
				Name: "AuditFinding",
				Fields: []db.EntityField{
					{Name: "id", Type: "string", Required: true, Comment: "Unique ID"},
					{Name: "severity", Type: "string", Required: true},
				},
			},
		},
		DependencyManifest: []db.Dependency{
			{Name: "@modelcontextprotocol/sdk", Version: "^1.12.0", Ecosystem: "npm", Purpose: "MCP framework"},
		},
		MCPTools: []db.MCPTool{
			{Name: "scan_codebase", Description: "Scan files"},
		},
		SecurityConsiderations: []db.SecurityItem{
			{Category: "deps", Description: "Minimal deps", Severity: "MEDIUM", MitigationStrategy: "Prefer native"},
		},
	}

	out := buildMADRDocument("Node Auditor MCP Server", session, bp)

	checks := []struct {
		label   string
		content string
	}{
		// YAML frontmatter
		{"frontmatter start", "---\nstatus: accepted"},
		{"session ID", "session: session-test-123"},
		{"tech stack", "tech_stack: Node"},
		{"risk level", "risk_level: medium"},
		{"schema version", "schema_version: 1"},

		// Persona
		{"persona section", "## Persona"},
		{"persona content", "Node.js and TypeScript"},

		// Context
		{"context section", "## Context and Problem Statement"},
		{"business case", "Audit Node.js codebases"},

		// Decision Drivers
		{"decision drivers", "## Decision Drivers"},

		// Architecture
		{"module hierarchy", "### Module Hierarchy"},
		{"file structure", "### File Structure"},
		{"data model", "### Data Model"},
		{"file path", "src/index.ts"},

		// Security
		{"security mandates", "## Security Mandates"},

		// ADR (MADR 4.0 format)
		{"ADR heading", "### ADR-1: ESLint Strategy"},
		{"ADR status", "**Status:** Accepted"},
		{"chosen option format", "Chosen option:"},
		{"pros cons section", "#### Pros and Cons of the Options"},
		{"good because", "Good, because"},
		{"bad because", "Bad, because"},
		{"rejected because", "Rejected, because"},
		{"confirmation", "##### Confirmation"},

		// Guardrails
		{"guardrails", "## Implementation Guardrails"},
		{"anti-patterns", "### Anti-Patterns (DO NOT)"},
		{"vulnerabilities", "### Known Vulnerabilities to Mitigate"},

		// Roadmap
		{"dependency manifest", "### Dependency Manifest"},
		{"mcp sdk", "@modelcontextprotocol/sdk"},

		// MCP Interfaces
		{"mcp tools", "### MCP Tools"},
		{"scan tool", "scan_codebase"},

		// Validation
		{"validation", "## Validation Criteria"},
	}

	for _, c := range checks {
		if !strings.Contains(out, c.content) {
			t.Errorf("Missing %s: expected %q in output", c.label, c.content)
		}
	}
}

func TestBuildMADRDocument_MinimalInput(t *testing.T) {
	session := &db.SessionState{
		SessionID: "session-minimal",
		TechStack: "Go",
	}
	bp := &db.Blueprint{
		ADRs: []db.ADR{
			{Title: "Use stdlib", Status: "Accepted", Context: "Minimize deps", Decision: "stdlib only", Consequences: "No third-party risk"},
		},
	}

	out := buildMADRDocument("Minimal Project", session, bp)

	if !strings.Contains(out, "# Minimal Project") {
		t.Error("Missing title")
	}
	if !strings.Contains(out, "ADR-1: Use stdlib") {
		t.Error("Missing ADR")
	}
	if !strings.Contains(out, "status: accepted") {
		t.Error("Missing YAML frontmatter")
	}
}
