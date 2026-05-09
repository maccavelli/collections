package integration

import (
	"fmt"
	"strings"
	"time"

	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/persona"
)

// buildMADRDocument generates a comprehensive MADR 4.0-compliant specification
// document optimized for LLM agent consumption. It serves as the authoritative
// source of truth for implementing the designed project from scratch.
func buildMADRDocument(title string, session *db.SessionState, bp *db.Blueprint) string {
	var b strings.Builder

	// --- YAML Frontmatter (MADR 4.0) ---
	b.WriteString("---\n")
	b.WriteString("status: accepted\n")
	b.WriteString(fmt.Sprintf("date: %s\n", time.Now().UTC().Format("2006-01-02")))
	if session.SessionID != "" {
		b.WriteString(fmt.Sprintf("session: %s\n", session.SessionID))
	}
	if session.JiraID != "" {
		b.WriteString(fmt.Sprintf("jira: %s\n", session.JiraID))
	}
	if session.TechStack != "" {
		b.WriteString(fmt.Sprintf("tech_stack: %s\n", session.TechStack))
	}
	if session.TargetEnvironment != "" {
		b.WriteString(fmt.Sprintf("target_environment: %s\n", session.TargetEnvironment))
	}
	if len(session.Labels) > 0 {
		b.WriteString(fmt.Sprintf("tags: [%s]\n", strings.Join(session.Labels, ", ")))
	}
	if len(session.ComplianceRequirements) > 0 {
		b.WriteString(fmt.Sprintf("compliance: [%s]\n", strings.Join(session.ComplianceRequirements, ", ")))
	}
	if session.RiskLevel != "" {
		b.WriteString(fmt.Sprintf("risk_level: %s\n", session.RiskLevel))
	}
	b.WriteString(fmt.Sprintf("schema_version: %d\n", db.CurrentSchemaVersion))
	b.WriteString("---\n\n")

	// --- Title ---
	b.WriteString(fmt.Sprintf("# %s\n\n", title))

	// --- Persona ---
	if p := persona.GeneratePersona(session, bp); p != "" {
		b.WriteString("## Persona\n\n")
		b.WriteString(fmt.Sprintf("> %s\n\n", p))
	}

	// --- Context and Problem Statement ---
	b.WriteString("## Context and Problem Statement\n\n")
	if session.BusinessCase != "" {
		b.WriteString(fmt.Sprintf("%s\n\n", session.BusinessCase))
	}
	if session.RefinedIdea != "" {
		b.WriteString(fmt.Sprintf("%s\n\n", session.RefinedIdea))
	} else if session.OriginalIdea != "" {
		b.WriteString(fmt.Sprintf("%s\n\n", session.OriginalIdea))
	}

	// --- Decision Drivers ---
	writeDecisionDrivers(&b, session)

	// --- Architecture Overview ---
	writeArchitectureOverview(&b, session, bp)

	// --- Security Mandates ---
	writeSecurityMandates(&b, session, bp)

	// --- Formal Decision Records (MADR 4.0 per-ADR) ---
	if len(bp.ADRs) > 0 {
		b.WriteString("## Formal Decision Records\n\n")
		for i, adr := range bp.ADRs {
			writeADR(&b, i+1, &adr)
		}
	}

	// --- Implementation Guardrails ---
	writeGuardrails(&b, session)

	// --- Technical Implementation Roadmap ---
	writeRoadmap(&b, bp)

	// --- Non-Functional Requirements ---
	if len(bp.NonFunctionalRequirements) > 0 {
		b.WriteString("## Non-Functional Requirements\n\n")
		b.WriteString("| Category | Requirement | Target | Priority |\n")
		b.WriteString("|----------|-------------|--------|----------|\n")
		for _, nfr := range bp.NonFunctionalRequirements {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", nfr.Category, nfr.Requirement, nfr.Target, nfr.Priority))
		}
		b.WriteString("\n")
	}

	// --- API Contracts / MCP Interfaces ---
	writeMCPInterfaces(&b, bp)

	// --- Standards References ---
	if session.DesignProposal != nil && len(session.DesignProposal.ReferencedStandards) > 0 {
		b.WriteString("## Standards References\n\n")
		b.WriteString("| Standard | Rule | Application |\n")
		b.WriteString("|----------|------|-------------|\n")
		for _, ref := range session.DesignProposal.ReferencedStandards {
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", ref.StandardURL, ref.Rule, ref.Application))
		}
		b.WriteString("\n")
	}

	// --- Validation Criteria ---
	writeValidationCriteria(&b, bp)

	return b.String()
}

func writeDecisionDrivers(b *strings.Builder, session *db.SessionState) {
	drivers := collectDrivers(session)
	if len(drivers) == 0 {
		return
	}
	b.WriteString("## Decision Drivers\n\n")
	for _, d := range drivers {
		b.WriteString(fmt.Sprintf("* %s\n", d))
	}
	b.WriteString("\n")
}

func collectDrivers(session *db.SessionState) []string {
	var drivers []string
	if session.DesignProposal != nil {
		for _, st := range session.DesignProposal.StackTuning {
			drivers = append(drivers, fmt.Sprintf("[%s] %s — %s", st.Priority, st.Recommendation, st.Rationale))
		}
	}
	if session.SynthesisResolution != nil {
		for _, dec := range session.SynthesisResolution.Decisions {
			drivers = append(drivers, fmt.Sprintf("%s — %s", dec.Topic, dec.Rationale))
		}
	}
	return drivers
}

func writeArchitectureOverview(b *strings.Builder, session *db.SessionState, bp *db.Blueprint) {
	hasModules := session.DesignProposal != nil && len(session.DesignProposal.ProposedModules) > 0
	hasFiles := len(bp.FileStructure) > 0
	hasData := len(bp.DataModel) > 0
	if !hasModules && !hasFiles && !hasData {
		return
	}

	b.WriteString("## Architecture Overview\n\n")

	// Narrative
	if session.DesignProposal != nil && session.DesignProposal.Narrative != "" {
		b.WriteString(fmt.Sprintf("%s\n\n", session.DesignProposal.Narrative))
	}

	// Module Hierarchy
	if hasModules {
		b.WriteString("### Module Hierarchy\n\n")
		b.WriteString("| Module | Purpose | Responsibilities | Dependencies |\n")
		b.WriteString("|--------|---------|-----------------|---------------|\n")
		for _, mod := range session.DesignProposal.ProposedModules {
			resp := strings.Join(mod.Responsibilities, ", ")
			deps := strings.Join(mod.Dependencies, ", ")
			if deps == "" {
				deps = "—"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", mod.Name, mod.Purpose, resp, deps))
		}
		b.WriteString("\n")
	}

	// File Structure
	if hasFiles {
		b.WriteString("### File Structure\n\n")
		b.WriteString("| Path | Type | Language | Description | Exports |\n")
		b.WriteString("|------|------|----------|-------------|--------|\n")
		for _, f := range bp.FileStructure {
			exports := strings.Join(f.Exports, ", ")
			if exports == "" {
				exports = "—"
			}
			lang := f.Language
			if lang == "" {
				lang = "—"
			}
			desc := f.Description
			if desc == "" {
				desc = "—"
			}
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n", f.Path, f.Type, lang, desc, exports))
		}
		b.WriteString("\n")
	}

	// Data Model
	if hasData {
		b.WriteString("### Data Model\n\n")
		for _, entity := range bp.DataModel {
			b.WriteString(fmt.Sprintf("#### %s\n\n", entity.Name))
			b.WriteString("| Field | Type | Required | Comment |\n")
			b.WriteString("|-------|------|----------|--------|\n")
			for _, field := range entity.Fields {
				req := "No"
				if field.Required {
					req = "Yes"
				}
				comment := field.Comment
				if comment == "" {
					comment = "—"
				}
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", field.Name, field.Type, req, comment))
			}
			if len(entity.Relationships) > 0 {
				b.WriteString(fmt.Sprintf("\n**Relationships:** %s\n", strings.Join(entity.Relationships, "; ")))
			}
			b.WriteString("\n")
		}
	}
}

func writeSecurityMandates(b *strings.Builder, session *db.SessionState, bp *db.Blueprint) {
	var items []db.SecurityItem
	if session.DesignProposal != nil {
		items = append(items, session.DesignProposal.SecurityMandates...)
	}
	items = append(items, bp.SecurityConsiderations...)
	if len(items) == 0 {
		return
	}
	b.WriteString("## Security Mandates\n\n")
	b.WriteString("| Severity | Category | Description | Mitigation Strategy |\n")
	b.WriteString("|----------|----------|-------------|--------------------|\n")
	for _, si := range items {
		b.WriteString(fmt.Sprintf("| **%s** | %s | %s | %s |\n", si.Severity, si.Category, si.Description, si.MitigationStrategy))
	}
	b.WriteString("\n")
}

func writeADR(b *strings.Builder, num int, adr *db.ADR) {
	b.WriteString(fmt.Sprintf("### ADR-%d: %s\n\n", num, adr.Title))
	b.WriteString(fmt.Sprintf("**Status:** %s", adr.Status))
	if adr.DecisionDate != "" {
		b.WriteString(fmt.Sprintf(" | **Date:** %s", adr.DecisionDate))
	}
	if len(adr.Tags) > 0 {
		b.WriteString(fmt.Sprintf(" | **Tags:** %s", strings.Join(adr.Tags, ", ")))
	}
	b.WriteString("\n\n")

	// Context and Problem Statement
	b.WriteString("#### Context and Problem Statement\n\n")
	b.WriteString(fmt.Sprintf("%s\n\n", adr.Context))

	// Decision Drivers
	if len(adr.DecisionDrivers) > 0 {
		b.WriteString("#### Decision Drivers\n\n")
		for _, driver := range adr.DecisionDrivers {
			b.WriteString(fmt.Sprintf("* %s\n", driver))
		}
		b.WriteString("\n")
	}

	// Considered Options
	if len(adr.Alternatives) > 0 {
		b.WriteString("#### Considered Options\n\n")
		b.WriteString(fmt.Sprintf("* **%s** (chosen)\n", adr.Decision))
		for _, alt := range adr.Alternatives {
			b.WriteString(fmt.Sprintf("* %s\n", alt.Name))
		}
		b.WriteString("\n")
	}

	// Decision Outcome (MADR 4.0 format)
	b.WriteString("#### Decision Outcome\n\n")
	if len(adr.Alternatives) > 0 {
		b.WriteString(fmt.Sprintf("Chosen option: \"%s\", because %s\n\n", adr.Decision, adr.Consequences))
	} else {
		b.WriteString(fmt.Sprintf("%s\n\n", adr.Decision))
	}

	// Consequences
	if adr.Consequences != "" {
		b.WriteString("##### Consequences\n\n")
		b.WriteString(fmt.Sprintf("* %s\n\n", adr.Consequences))
	}

	// Confirmation
	if adr.Confirmation != "" {
		b.WriteString("##### Confirmation\n\n")
		b.WriteString(fmt.Sprintf("%s\n\n", adr.Confirmation))
	}

	// Pros and Cons of the Options (MADR 4.0)
	if len(adr.Alternatives) > 0 {
		b.WriteString("#### Pros and Cons of the Options\n\n")
		for _, alt := range adr.Alternatives {
			b.WriteString(fmt.Sprintf("##### %s\n\n", alt.Name))
			if alt.Pros != "" {
				b.WriteString(fmt.Sprintf("* Good, because %s\n", alt.Pros))
			}
			if alt.Cons != "" {
				b.WriteString(fmt.Sprintf("* Bad, because %s\n", alt.Cons))
			}
			if alt.RejectionReason != "" {
				b.WriteString(fmt.Sprintf("* Rejected, because %s\n", alt.RejectionReason))
			}
			b.WriteString("\n")
		}
	}

	// Extended sections
	if adr.ComplianceCheck != "" {
		b.WriteString(fmt.Sprintf("#### Compliance Check\n\n%s\n\n", adr.ComplianceCheck))
	}
	if adr.SecurityFootprint != "" {
		b.WriteString(fmt.Sprintf("#### Security Footprint\n\n%s\n\n", adr.SecurityFootprint))
	}

	b.WriteString("---\n\n")
}

func writeGuardrails(b *strings.Builder, session *db.SessionState) {
	if session.SkepticAnalysis == nil {
		return
	}
	hasConcerns := len(session.SkepticAnalysis.DesignConcerns) > 0
	hasVulns := len(session.SkepticAnalysis.Vulnerabilities) > 0
	hasUnresolved := session.SynthesisResolution != nil && len(session.SynthesisResolution.UnresolvedDependencies) > 0
	hasQuestions := session.SynthesisResolution != nil && len(session.SynthesisResolution.OutstandingQuestions) > 0

	if !hasConcerns && !hasVulns && !hasUnresolved && !hasQuestions {
		return
	}

	b.WriteString("## Implementation Guardrails\n\n")

	if hasConcerns {
		b.WriteString("### Anti-Patterns (DO NOT)\n\n")
		for _, dc := range session.SkepticAnalysis.DesignConcerns {
			b.WriteString(fmt.Sprintf("* **[%s] %s:** DO NOT %s", dc.Severity, dc.Area, dc.Concern))
			if dc.Suggestion != "" {
				b.WriteString(fmt.Sprintf(" — Instead: %s", dc.Suggestion))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if hasVulns {
		b.WriteString("### Known Vulnerabilities to Mitigate\n\n")
		b.WriteString("| Severity | Category | Description | Required Mitigation |\n")
		b.WriteString("|----------|----------|-------------|--------------------|\n")
		for _, v := range session.SkepticAnalysis.Vulnerabilities {
			b.WriteString(fmt.Sprintf("| **%s** | %s | %s | %s |\n", v.Severity, v.Category, v.Description, v.MitigationStrategy))
		}
		b.WriteString("\n")
	}

	if hasUnresolved {
		b.WriteString("### Unresolved Dependencies\n\n")
		for _, dep := range session.SynthesisResolution.UnresolvedDependencies {
			b.WriteString(fmt.Sprintf("* %s\n", dep))
		}
		b.WriteString("\n")
	}

	if hasQuestions {
		b.WriteString("### Outstanding Questions\n\n")
		for _, oq := range session.SynthesisResolution.OutstandingQuestions {
			b.WriteString(fmt.Sprintf("* **%s:** %s (Impact: %s)\n", oq.Topic, oq.Question, oq.Impact))
		}
		b.WriteString("\n")
	}
}

func writeRoadmap(b *strings.Builder, bp *db.Blueprint) {
	hasStrategy := len(bp.ImplementationStrategy) > 0
	hasDeps := len(bp.DependencyManifest) > 0
	hasComplexity := len(bp.ComplexityScores) > 0
	hasTesting := len(bp.TestingStrategy) > 0

	if !hasStrategy && !hasDeps && !hasComplexity && !hasTesting {
		return
	}

	b.WriteString("## Technical Implementation Roadmap\n\n")

	if hasDeps {
		b.WriteString("### Dependency Manifest\n\n")
		b.WriteString("| Package | Version | Ecosystem | Purpose | Dev Only |\n")
		b.WriteString("|---------|---------|-----------|---------|----------|\n")
		for _, dep := range bp.DependencyManifest {
			devOnly := "No"
			if dep.DevOnly {
				devOnly = "Yes"
			}
			purpose := dep.Purpose
			if purpose == "" {
				purpose = "—"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", dep.Name, dep.Version, dep.Ecosystem, purpose, devOnly))
		}
		b.WriteString("\n")
	}

	if hasStrategy {
		b.WriteString("### Strategy Mapping\n\n")
		b.WriteString("| Requirement | Implementation Pattern |\n")
		b.WriteString("|-------------|----------------------|\n")
		for req, pattern := range bp.ImplementationStrategy {
			b.WriteString(fmt.Sprintf("| %s | %s |\n", req, pattern))
		}
		b.WriteString("\n")
	}

	if hasComplexity {
		b.WriteString("### Complexity Estimation\n\n")
		b.WriteString("| Feature | Story Points |\n")
		b.WriteString("|---------|-------------|\n")
		totalPoints := 0
		for feature, points := range bp.ComplexityScores {
			b.WriteString(fmt.Sprintf("| %s | %d |\n", feature, points))
			totalPoints += points
		}
		b.WriteString(fmt.Sprintf("| **Total** | **%d** |\n\n", totalPoints))
	}

	if hasTesting {
		b.WriteString("### Testing Strategy\n\n")
		b.WriteString("| Feature | Test Approach |\n")
		b.WriteString("|---------|--------------|\n")
		for feature, approach := range bp.TestingStrategy {
			b.WriteString(fmt.Sprintf("| %s | %s |\n", feature, approach))
		}
		b.WriteString("\n")
	}
}

func writeMCPInterfaces(b *strings.Builder, bp *db.Blueprint) {
	hasTools := len(bp.MCPTools) > 0
	hasResources := len(bp.MCPResources) > 0
	hasPrompts := len(bp.MCPPrompts) > 0
	hasAPI := len(bp.APIContracts) > 0

	if !hasTools && !hasResources && !hasPrompts && !hasAPI {
		return
	}

	b.WriteString("## API Contracts / MCP Interfaces\n\n")

	if hasTools {
		b.WriteString("### MCP Tools\n\n")
		b.WriteString("| Tool | Description | Input Schema |\n")
		b.WriteString("|------|-------------|-------------|\n")
		for _, tool := range bp.MCPTools {
			schema := tool.InputSchema
			if schema == "" {
				schema = "—"
			}
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", tool.Name, tool.Description, schema))
		}
		b.WriteString("\n")
	}

	if hasResources {
		b.WriteString("### MCP Resources\n\n")
		b.WriteString("| URI | Name | Description |\n")
		b.WriteString("|-----|------|-------------|\n")
		for _, res := range bp.MCPResources {
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", res.URI, res.Name, res.Description))
		}
		b.WriteString("\n")
	}

	if hasPrompts {
		b.WriteString("### MCP Prompts\n\n")
		b.WriteString("| Name | Description | Arguments |\n")
		b.WriteString("|------|-------------|----------|\n")
		for _, p := range bp.MCPPrompts {
			args := strings.Join(p.Arguments, ", ")
			if args == "" {
				args = "—"
			}
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", p.Name, p.Description, args))
		}
		b.WriteString("\n")
	}

	if hasAPI {
		b.WriteString("### REST Endpoints\n\n")
		b.WriteString("| Method | Path | Description |\n")
		b.WriteString("|--------|------|-------------|\n")
		for _, ep := range bp.APIContracts {
			desc := ep.Description
			if desc == "" {
				desc = "—"
			}
			b.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n", ep.Method, ep.Path, desc))
		}
		b.WriteString("\n")
	}
}

func writeValidationCriteria(b *strings.Builder, bp *db.Blueprint) {
	var criteria []string
	for _, adr := range bp.ADRs {
		if adr.Confirmation != "" {
			criteria = append(criteria, fmt.Sprintf("**%s:** %s", adr.Title, adr.Confirmation))
		}
	}
	for feature, approach := range bp.TestingStrategy {
		criteria = append(criteria, fmt.Sprintf("**%s:** %s", feature, approach))
	}
	for _, nfr := range bp.NonFunctionalRequirements {
		criteria = append(criteria, fmt.Sprintf("**%s:** %s must achieve %s", nfr.Category, nfr.Requirement, nfr.Target))
	}
	if len(criteria) == 0 {
		return
	}
	b.WriteString("## Validation Criteria\n\n")
	for _, c := range criteria {
		b.WriteString(fmt.Sprintf("* %s\n", c))
	}
	b.WriteString("\n")
}
