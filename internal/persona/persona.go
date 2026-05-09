// Package persona generates technical SME personas from session metadata.
package persona

import (
	"fmt"
	"strings"

	"mcp-server-magicdev/internal/db"
)

// stackExpertise maps tech stacks to domain expertise descriptions.
var stackExpertise = map[string]string{
	"node":   "Node.js and TypeScript ecosystem, including modern ESM patterns, native Node.js APIs, and npm package management",
	"go":     "Go systems programming, including concurrency patterns, stdlib-first design, and idiomatic error handling",
	".net":   ".NET platform architecture, including C#, ASP.NET Core, Entity Framework, and NuGet package management",
	"python": "Python ecosystem, including async/await patterns, type annotations, and pip/poetry dependency management",
}

// envExpertise maps target environments to deployment context descriptions.
var envExpertise = map[string]string{
	"local-ide":     "local development tooling that runs as IDE-managed child processes",
	"containerized": "containerized microservices deployed via Docker and Kubernetes",
	"cloud":         "cloud-native architectures leveraging managed services and serverless patterns",
	"on-prem":       "on-premises enterprise deployments with strict network and compliance boundaries",
	"hybrid":        "hybrid cloud/on-prem architectures requiring portable, environment-agnostic design",
	"edge":          "edge computing with constrained resources, offline-first design, and eventual consistency",
}

// GeneratePersona produces a technical SME persona paragraph derived from session
// metadata. The persona establishes the expert perspective an LLM coding agent
// should adopt when implementing the project — including technology expertise,
// domain knowledge, security posture, and architectural priorities.
func GeneratePersona(session *db.SessionState, bp *db.Blueprint) string {
	if session == nil {
		return ""
	}

	var parts []string

	// Primary technology expertise
	stack := strings.ToLower(session.TechStack)
	expertise, ok := stackExpertise[stack]
	if !ok {
		expertise = fmt.Sprintf("%s development", session.TechStack)
	}

	// Domain expertise from labels
	var domains []string
	for _, label := range session.Labels {
		if strings.HasPrefix(label, "domain:") {
			domain := strings.TrimPrefix(label, "domain:")
			domains = append(domains, strings.ReplaceAll(domain, "-", " "))
		}
	}
	domainStr := ""
	if len(domains) > 0 {
		domainStr = fmt.Sprintf(", with specialization in %s", strings.Join(domains, ", "))
	}

	parts = append(parts, fmt.Sprintf(
		"You are a senior software architect with deep expertise in %s%s.",
		expertise, domainStr,
	))

	// Deployment context
	env := strings.ToLower(session.TargetEnvironment)
	if envDesc, ok := envExpertise[env]; ok {
		parts = append(parts, fmt.Sprintf(
			"You design systems for %s.", envDesc,
		))
	}

	// Security posture
	if session.DesignProposal != nil && len(session.DesignProposal.SecurityMandates) > 0 {
		var categories []string
		for _, sm := range session.DesignProposal.SecurityMandates {
			categories = append(categories, sm.Category)
		}
		parts = append(parts, fmt.Sprintf(
			"You approach implementation with a security-first mindset, with particular focus on: %s.",
			strings.Join(categories, ", "),
		))
	}

	// Compliance requirements
	if len(session.ComplianceRequirements) > 0 {
		parts = append(parts, fmt.Sprintf(
			"All implementations must comply with %s regulatory frameworks.",
			strings.Join(session.ComplianceRequirements, ", "),
		))
	}

	// Stack tuning priorities
	if session.DesignProposal != nil && len(session.DesignProposal.StackTuning) > 0 {
		var priorities []string
		for _, st := range session.DesignProposal.StackTuning {
			if strings.EqualFold(st.Priority, "HIGH") || strings.EqualFold(st.Priority, "must-have") {
				priorities = append(priorities, strings.ToLower(st.Recommendation))
			}
		}
		if len(priorities) > 0 {
			parts = append(parts, fmt.Sprintf(
				"Your implementation approach prioritizes: %s.",
				strings.Join(priorities, "; "),
			))
		}
	}

	// Module expertise
	if session.DesignProposal != nil && len(session.DesignProposal.ProposedModules) > 0 {
		var moduleNames []string
		for _, mod := range session.DesignProposal.ProposedModules {
			moduleNames = append(moduleNames, mod.Name)
		}
		parts = append(parts, fmt.Sprintf(
			"You are responsible for the complete implementation of the following modules: %s.",
			strings.Join(moduleNames, ", "),
		))
	}

	return strings.Join(parts, " ")
}
