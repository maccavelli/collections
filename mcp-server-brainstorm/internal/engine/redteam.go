package engine

import (
	"context"
	"fmt"
	"strings"

	"mcp-server-brainstorm/internal/models"
)

// hasASTPresence intelligently scans the traceMap for empirical evidence.
// In standalone mode (traceMap == nil), it falls back to true to allow textual heuristics.
func hasASTPresence(traceMap map[string]interface{}, keywords ...string) bool {
	if traceMap == nil {
		return true // Standalone Mode: Always true, relying on textual heuristics
	}
	rawStr := fmt.Sprintf("%v", traceMap)
	for _, kw := range keywords {
		if strings.Contains(rawStr, kw) {
			return true
		}
	}
	return false
}

// RedTeamReview simulates adversarial personas to
// challenge the design from multiple angles. Returns
// compact structured challenges.
func (e *Engine) RedTeamReview(
	ctx context.Context, design string, traceMap map[string]interface{},
) ([]models.RedTeamChallenge, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	lower := strings.ToLower(design)
	var out []models.RedTeamChallenge

	// Run all persona checks
	if c := e.checkMaintenancePersona(lower, traceMap); c != nil {
		out = append(out, c...)
	}
	if c := e.checkSecurityPersona(lower, traceMap); c != nil {
		out = append(out, c...)
	}
	if c := e.checkScalabilityPersona(lower, traceMap); c != nil {
		out = append(out, c...)
	}
	if c := e.checkCompatibilityPersona(lower, traceMap); c != nil {
		out = append(out, c...)
	}
	if c := e.checkReliabilityPersona(lower, traceMap); c != nil {
		out = append(out, c...)
	}
	if c := e.checkPerformancePersona(lower, traceMap); c != nil {
		out = append(out, c...)
	}
	if c := e.checkCompliancePersona(lower, traceMap); c != nil {
		out = append(out, c...)
	}
	if c := e.checkOperationsPersona(lower, traceMap); c != nil {
		out = append(out, c...)
	}

	// Fallback logic severely reduced to avoid purely additive false positives in standalone mode.
	if len(out) == 0 && traceMap == nil {
		// In standalone mode, if the text is wildly sparse, we emit one soft nudge instead of hard blockers.
		if len(strings.Fields(lower)) < 10 {
			out = append(out, models.RedTeamChallenge{
				Persona:  "Clarity",
				Question: "Design is extremely sparse. Are there any hidden edges?",
			})
		}
	}

	return out, nil
}

func (e *Engine) checkMaintenancePersona(design string, traceMap map[string]interface{}) []models.RedTeamChallenge {
	// If AST has logging/metrics injected, bypass this challenge completely.
	if traceMap != nil && hasASTPresence(traceMap, "log", "zap", "prometheus", "telemetry") {
		return nil
	}
	if !strings.Contains(design, "log") &&
		!strings.Contains(design, "debug") &&
		!strings.Contains(design, "monitor") {
		return []models.RedTeamChallenge{{
			Persona:  "Maintenance",
			Question: "How do you debug this at 3 AM?",
		}}
	}
	return nil
}

func (e *Engine) checkSecurityPersona(design string, traceMap map[string]interface{}) []models.RedTeamChallenge {
	// Disable Security bounds checking if no HTTP/Auth logic is physically present
	if traceMap != nil && !hasASTPresence(traceMap, "net/http", "crypto", "auth", "token", "password", "context") {
		return nil
	}

	if strings.Contains(design, "api") ||
		strings.Contains(design, "token") ||
		strings.Contains(design, "secret") ||
		strings.Contains(design, "password") {
		return []models.RedTeamChallenge{{
			Persona:  "Security",
			Question: "What if the API key leaks? Is data encrypted at rest?",
		}}
	}
	return nil
}

func (e *Engine) checkScalabilityPersona(design string, traceMap map[string]interface{}) []models.RedTeamChallenge {
	// Only run scalability checks if network or DB layer is detected
	if traceMap != nil && !hasASTPresence(traceMap, "net/http", "database/sql", "grpc") {
		return nil
	}

	if strings.Contains(design, "single") ||
		strings.Contains(design, "monolith") ||
		strings.Contains(design, "bottleneck") ||
		!strings.Contains(design, "scale") {
		return []models.RedTeamChallenge{{
			Persona:  "Scalability",
			Question: "What if load triples? Where is the bottleneck?",
		}}
	}
	return nil
}

func (e *Engine) checkCompatibilityPersona(design string, traceMap map[string]interface{}) []models.RedTeamChallenge {
	if strings.Contains(design, "change") ||
		strings.Contains(design, "migrat") ||
		strings.Contains(design, "deprecat") {
		return []models.RedTeamChallenge{{
			Persona:  "Compatibility",
			Question: "Will existing clients break?",
		}}
	}
	return nil
}

func (e *Engine) checkReliabilityPersona(design string, traceMap map[string]interface{}) []models.RedTeamChallenge {
	if traceMap != nil && !hasASTPresence(traceMap, "net/http", "database", "net", "grpc", "io") {
		return nil // Pure logic functions rarely need circuit breakers.
	}

	if !strings.Contains(design, "retry") &&
		!strings.Contains(design, "fallback") &&
		!strings.Contains(design, "circuit break") {
		return []models.RedTeamChallenge{{
			Persona:  "Reliability",
			Question: "What is the failure recovery strategy?",
		}}
	}
	return nil
}

func (e *Engine) checkPerformancePersona(design string, traceMap map[string]interface{}) []models.RedTeamChallenge {
	if strings.Contains(design, "loop") ||
		strings.Contains(design, "recursive") ||
		strings.Contains(design, "search") {
		return []models.RedTeamChallenge{{
			Persona:  "Performance",
			Question: "How does this scale with O(n) or O(n^2)? Potential bottlenecks?",
		}}
	}
	return nil
}

func (e *Engine) checkCompliancePersona(design string, traceMap map[string]interface{}) []models.RedTeamChallenge {
	// Base persona only enabled if persistence or tracking is detected via AST.
	if traceMap != nil && !hasASTPresence(traceMap, "database", "store", "os", "file") {
		return nil
	}

	if strings.Contains(design, "log") ||
		strings.Contains(design, "persist") ||
		strings.Contains(design, "store") {
		return []models.RedTeamChallenge{{
			Persona:  "Compliance",
			Question: "Are we logging sensitive data? Is it GDPR/PII compliant?",
		}}
	}
	return nil
}

func (e *Engine) checkOperationsPersona(design string, traceMap map[string]interface{}) []models.RedTeamChallenge {
	if traceMap != nil && !hasASTPresence(traceMap, "main", "deploy", "kubernetes", "cmd") {
		return nil
	}

	if strings.Contains(design, "deploy") ||
		strings.Contains(design, "update") ||
		strings.Contains(design, "config") {
		return []models.RedTeamChallenge{{
			Persona:  "Operations",
			Question: "Can this be rolled back? Are there health checks?",
		}}
	}
	return nil
}
