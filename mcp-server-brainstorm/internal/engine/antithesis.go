package engine

import (
	"context"
	"fmt"
	"os"
	"strings"

	"mcp-server-brainstorm/internal/models"
)

// GenerateCounterThesis challenges a thesis proposal across the same 6 unified
// pillars, scored from the risk perspective. In standalone mode it uses keyword
// heuristics; in orchestrator mode it delegates to existing sub-engines and
// leverages all available traceMap keys.
func (e *Engine) GenerateCounterThesis(
	ctx context.Context,
	thesisText string,
	standards string,
	traceMap map[string]any,
) (models.CounterThesisReport, error) {
	select {
	case <-ctx.Done():
		return models.CounterThesisReport{}, ctx.Err()
	default:
	}

	lower := strings.ToLower(thesisText)
	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"

	pillars := make([]models.DialecticPillar, 0, 6)
	pillars = append(pillars, e.skepticTypeSafety(lower, traceMap))
	pillars = append(pillars, e.skepticModernization(ctx, lower, traceMap, isOrchestrator))
	pillars = append(pillars, e.skepticModularization(lower, traceMap))
	pillars = append(pillars, e.skepticEfficiency(lower, traceMap))
	pillars = append(pillars, e.skepticReliability(lower, traceMap))
	pillars = append(pillars, e.skepticMaintainability(ctx, lower, traceMap, isOrchestrator))

	verdict := computeSkepticVerdict(pillars)

	report := models.CounterThesisReport{
		Summary: fmt.Sprintf(
			"Counter-Thesis: %d dimensions evaluated. Verdict: %s",
			len(pillars), verdict,
		),
		Verdict: verdict,
		Pillars: pillars,
	}

	// Populate embedded AporiaReport fields for backwards compat.
	report.GenericBloat = extractPillarFinding(pillars, pillarNames[0])
	report.GreenTeaLocality = extractPillarFinding(pillars, pillarNames[3])

	suggestions := make([]string, 0, len(pillars))
	for _, p := range pillars {
		if p.Score < 7 {
			suggestions = append(suggestions, p.Finding)
		}
	}
	report.RedTeamSuggestions = suggestions
	report.RedTeamScore = computeAvgScore(pillars)
	report.RedTeamVerdict = buildCounterNarrative(pillars, standards)

	return report, nil
}

// --- Pillar 1: Type Safety & Generics (risk perspective) ---

func (e *Engine) skepticTypeSafety(
	text string, traceMap map[string]any,
) models.DialecticPillar {
	score := 7 // Benefit of the doubt
	var findings []string

	if strings.Contains(text, "type") && strings.Count(text, "[") > 2 {
		score -= 2
		findings = append(findings, "Deep type parameter nesting — potential 'Type Gymnastics' overhead")
	}

	interfaceCount := strings.Count(text, "interface")
	if interfaceCount > 3 {
		score -= 2
		findings = append(findings, fmt.Sprintf("Interface proliferation: %d references — evaluate consolidation", interfaceCount))
	}

	if strings.Contains(text, "wrapper") || strings.Contains(text, "adapter") ||
		strings.Contains(text, "proxy") || strings.Contains(text, "delegate") {
		score -= 1
		findings = append(findings, "Indirection layer detected — ensure abstraction pays for itself")
	}

	// AST enrichment
	if traceMap != nil {
		if interfaces, ok := traceMap["interfaces"].([]any); ok && len(interfaces) > 8 {
			score -= 1
			findings = append(findings, "AST: very high interface count — abstraction tax risk")
		}
	}

	return dialecticPillar(pillarNames[0], score, findings, "Type safety changes are within acceptable risk bounds.")
}

// --- Pillar 2: Modernization (risk perspective) ---

func (e *Engine) skepticModernization(
	ctx context.Context, text string, traceMap map[string]any, isOrchestrator bool,
) models.DialecticPillar {
	score := 7
	var findings []string

	hasRefactorIntent := strings.Contains(text, "refactor") || strings.Contains(text, "rewrite") ||
		strings.Contains(text, "moderniz")
	hasDeficiency := strings.Contains(text, "bug") || strings.Contains(text, "fix") ||
		strings.Contains(text, "error") || strings.Contains(text, "vulnerability") ||
		strings.Contains(text, "crash") || strings.Contains(text, "incorrect")

	if hasRefactorIntent && !hasDeficiency {
		score -= 3
		findings = append(findings, "Proposes refactoring without citing concrete deficiency — speculative change risk")
	}

	hasStyleOnly := (strings.Contains(text, "ergonomic") || strings.Contains(text, "idiomatic") ||
		strings.Contains(text, "cleaner")) && !strings.Contains(text, "safety") && !strings.Contains(text, "correctness")
	if hasStyleOnly {
		score -= 2
		findings = append(findings, "Change motivated by style/ergonomics rather than safety or correctness")
	}

	// Orchestrator: delegate to ChallengeAssumption for YAGNI validation
	if isOrchestrator && traceMap != nil {
		challenges, err := e.ChallengeAssumption(ctx, text, traceMap)
		if err == nil && len(challenges) > 2 {
			score -= 1
			findings = append(findings, "Engine: multiple assumption challenges raised — YAGNI concerns")
		}
	}

	return dialecticPillar(pillarNames[1], score, findings, "Modernization changes are justified.")
}

// --- Pillar 3: Modularization (risk perspective) ---

func (e *Engine) skepticModularization(
	text string, traceMap map[string]any,
) models.DialecticPillar {
	score := 7
	var findings []string

	if strings.Contains(text, "split") || strings.Contains(text, "extract") ||
		strings.Contains(text, "decompos") {
		score -= 1
		findings = append(findings, "Proposes decomposition — verify proportional cohesion improvement")
	}

	if strings.Contains(text, "package") && strings.Contains(text, "new") {
		score -= 1
		findings = append(findings, "New package creation — additional import complexity")
	}

	// AST enrichment: cross-package dependency depth
	if traceMap != nil {
		if deps, ok := traceMap["dependencies"].(string); ok && strings.Count(deps, "/") > 10 {
			score -= 1
			findings = append(findings, "AST: deep dependency chains — further splitting increases fragmentation")
		}
		if funcs, ok := traceMap["total_functions"].(float64); ok && funcs < 10 {
			score -= 1
			findings = append(findings, "AST: low function count — splitting a small package is premature")
		}
	}

	return dialecticPillar(pillarNames[2], score, findings, "Modularization impact is acceptable.")
}

// --- Pillar 4: Efficiency (risk perspective) ---

func (e *Engine) skepticEfficiency(
	text string, traceMap map[string]any,
) models.DialecticPillar {
	score := 7
	var findings []string

	if strings.Contains(text, "interface{}") || strings.Contains(text, "any") {
		score -= 1
		findings = append(findings, "interface{}/any usage forces boxing — potential heap allocation overhead")
	}
	if strings.Contains(text, "reflect") {
		score -= 2
		findings = append(findings, "Reflection introduces runtime type resolution overhead")
	}
	if strings.Contains(text, "append") && !strings.Contains(text, "cap") && !strings.Contains(text, "make") {
		score -= 1
		findings = append(findings, "append without pre-allocation — risk of GC pressure in loops")
	}

	// AST enrichment
	if traceMap != nil {
		if imports, ok := traceMap["imports"].([]any); ok {
			for _, imp := range imports {
				if s, ok := imp.(string); ok && strings.Contains(s, "reflect") {
					score -= 1
					findings = append(findings, "AST: reflect import confirmed — runtime cost validated")
					break
				}
			}
		}
	}

	return dialecticPillar(pillarNames[3], score, findings, "No significant runtime regression risk detected.")
}

// --- Pillar 5: Reliability (risk perspective) ---

func (e *Engine) skepticReliability(
	text string, traceMap map[string]any,
) models.DialecticPillar {
	score := 7
	var findings []string

	if strings.Contains(text, "mutex") || strings.Contains(text, "sync.") || strings.Contains(text, "rwmutex") {
		score -= 2
		findings = append(findings, "New sync primitives — concurrency complexity escalation")
	}
	if strings.Contains(text, "chan ") || strings.Contains(text, "channel") {
		score -= 1
		findings = append(findings, "New channel patterns — evaluate deadlock and goroutine leak risk")
	}
	if strings.Count(text, "wrap") > 2 {
		score -= 1
		findings = append(findings, "Deep error wrapping chains — risk of unreadable error traces")
	}

	// AST enrichment
	if traceMap != nil {
		if complexity, ok := traceMap["complexity"].([]any); ok {
			for _, c := range complexity {
				if cv, ok := c.(float64); ok && cv > 12 {
					score -= 2
					findings = append(findings, "AST: cyclomatic complexity near ceiling — additions risky")
					break
				}
			}
		}
		if conc, ok := traceMap["concurrency"].(string); ok && strings.Contains(conc, "leak") {
			score -= 1
			findings = append(findings, "AST: existing goroutine leak indicators — adding complexity dangerous")
		}
	}

	return dialecticPillar(pillarNames[4], score, findings, "Reliability impact is within acceptable bounds.")
}

// --- Pillar 6: Maintainability (risk perspective) ---

func (e *Engine) skepticMaintainability(
	ctx context.Context, text string, traceMap map[string]any, isOrchestrator bool,
) models.DialecticPillar {
	score := 7
	var findings []string

	if strings.Contains(text, "export") || strings.Contains(text, "public api") || strings.Contains(text, "signature") {
		score -= 2
		findings = append(findings, "Exported API signature changes — breaking change potential")
	}
	if strings.Contains(text, "go.mod") || strings.Contains(text, "dependency") ||
		(strings.Contains(text, "import") && strings.Contains(text, "new")) {
		score -= 1
		findings = append(findings, "New dependency additions — supply chain and build stability risk")
	}
	if strings.Contains(text, "json:") || strings.Contains(text, "yaml:") || strings.Contains(text, "serializ") {
		score -= 1
		findings = append(findings, "Serialization tag changes — data format compatibility risk")
	}
	if strings.Contains(text, "main.go") || strings.Contains(text, "initialization") || strings.Contains(text, "startup") {
		score -= 1
		findings = append(findings, "Changes to server initialization path — high blast radius")
	}

	// AST enrichment
	if traceMap != nil {
		if nodes, ok := traceMap["total_nodes"].(float64); ok && nodes > 500 {
			score -= 1
			findings = append(findings, "AST: large node count — blast radius is amplified")
		}
	}

	// Orchestrator: delegate to AnalyzeEvolution for empirical blast radius
	if isOrchestrator && traceMap != nil {
		evo, err := e.AnalyzeEvolution(ctx, text, "", traceMap)
		if err == nil && evo.Data.RiskLevel == "HIGH" {
			score -= 2
			findings = append(findings, "Engine: evolution analysis rates change as HIGH risk")
		}
	}

	return dialecticPillar(pillarNames[5], score, findings, "Maintainability risk is within acceptable bounds.")
}

// --- Shared helpers ---

func computeSkepticVerdict(pillars []models.DialecticPillar) string {
	if len(pillars) == 0 {
		return "REVIEW"
	}
	minScore := 10
	for _, p := range pillars {
		if p.Score < minScore {
			minScore = p.Score
		}
	}
	switch {
	case minScore >= 7:
		return "APPROVE"
	case minScore >= 4:
		return "REVIEW"
	default:
		return "REJECT"
	}
}

func buildCounterNarrative(pillars []models.DialecticPillar, standards string) string {
	var b strings.Builder
	b.WriteString("## Counter-Thesis: Performance & Robustness Audit\n\n")
	if standards != "" {
		b.WriteString("**Standards Applied**: ")
		if len(standards) > 100 {
			b.WriteString(standards[:100])
			b.WriteString("...")
		} else {
			b.WriteString(standards)
		}
		b.WriteString("\n\n")
	}
	for _, p := range pillars {
		fmt.Fprintf(&b, "### %s (Score: %d/10)\n%s\n\n", p.Name, p.Score, p.Finding)
	}
	return b.String()
}

func extractPillarFinding(pillars []models.DialecticPillar, name string) string {
	for _, p := range pillars {
		if p.Name == name {
			return p.Finding
		}
	}
	return ""
}

func computeAvgScore(pillars []models.DialecticPillar) int {
	if len(pillars) == 0 {
		return 0
	}
	total := 0
	for _, p := range pillars {
		total += p.Score
	}
	return total / len(pillars)
}
