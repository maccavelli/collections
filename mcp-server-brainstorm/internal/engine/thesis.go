package engine

import (
	"context"
	"fmt"
	"os"
	"strings"

	"mcp-server-brainstorm/internal/models"
)

// pillarNames defines the 6 unified pillar names shared by thesis and antithesis.
var pillarNames = [6]string{
	"Type Safety & Generics",
	"Modernization",
	"Modularization",
	"Efficiency",
	"Reliability",
	"Maintainability",
}

// GenerateThesis analyzes a project context and scores opportunity
// across 6 unified modernization pillars. In standalone mode
// (traceMap == nil), it relies on textual keyword heuristics.
// In orchestrator mode, AST trace data and sub-engine delegation
// enrich the scoring.
func (e *Engine) GenerateThesis(
	ctx context.Context,
	projectContext string,
	standards string,
	traceMap map[string]interface{},
) (models.ThesisDocument, error) {
	select {
	case <-ctx.Done():
		return models.ThesisDocument{}, ctx.Err()
	default:
	}

	lower := strings.ToLower(projectContext)
	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"

	pillars := make([]models.DialecticPillar, 0, 6)
	pillars = append(pillars, e.thesisTypeSafety(lower, traceMap))
	pillars = append(pillars, e.thesisModernization(lower, traceMap))
	pillars = append(pillars, e.thesisModularization(ctx, lower, traceMap, isOrchestrator))
	pillars = append(pillars, e.thesisEfficiency(lower, traceMap))
	pillars = append(pillars, e.thesisReliability(lower, traceMap))
	pillars = append(pillars, e.thesisMaintainability(lower, traceMap))

	verdict := computeThesisVerdict(pillars)
	narrative := buildThesisNarrative(pillars, standards)

	doc := models.ThesisDocument{
		Summary: fmt.Sprintf(
			"Thesis: %d pillars evaluated. Verdict: %s",
			len(pillars), verdict,
		),
		Verdict: verdict,
	}
	doc.Data.Narrative = narrative
	doc.Data.Pillars = pillars
	if standards != "" {
		doc.Data.Standards = standards
	}

	return doc, nil
}

// --- Pillar 1: Type Safety & Generics ---

func (e *Engine) thesisTypeSafety(
	text string, traceMap map[string]interface{},
) models.DialecticPillar {
	score := 5
	var findings []string

	if strings.Contains(text, "interface{}") || strings.Contains(text, "any") {
		score += 2
		findings = append(findings, "interface{}/any usage — candidate for generic type parameters")
	}
	if strings.Contains(text, "type assertion") || strings.Contains(text, "type switch") {
		score += 1
		findings = append(findings, "Type assertions found — generics could enforce compile-time safety")
	}
	if strings.Contains(text, "reflect") {
		score += 1
		findings = append(findings, "Reflection detected — generics can eliminate runtime type resolution")
	}
	if strings.Contains(text, "errors.as") || strings.Contains(text, "errors.is") {
		score += 1
		findings = append(findings, "errors.As/Is usage — evaluate upgrade to errors.AsType[T]")
	}

	// AST enrichment
	if traceMap != nil {
		if interfaces, ok := traceMap["interfaces"].([]interface{}); ok && len(interfaces) > 5 {
			score += 1
			findings = append(findings, "AST: high interface count — generic constraint candidates")
		}
		if errPat, ok := traceMap["error_patterns"].(string); ok && strings.Contains(errPat, "assertion") {
			score += 1
			findings = append(findings, "AST: manual error type assertions confirmed")
		}
	}

	return dialecticPillar(pillarNames[0], score, findings, "No significant type safety improvements identified.")
}

// --- Pillar 2: Modernization ---

func (e *Engine) thesisModernization(
	text string, traceMap map[string]interface{},
) models.DialecticPillar {
	score := 5
	var findings []string

	if strings.Contains(text, "omitempty") {
		score += 2
		findings = append(findings, "omitempty tags found — evaluate migration to omitzero")
	}
	if strings.Contains(text, "&tmp") || (strings.Contains(text, "var ") && strings.Contains(text, "return &")) {
		score += 1
		findings = append(findings, "Temporary variable + pointer pattern — new(expr) can simplify")
	}
	if strings.Contains(text, "strings.index") || strings.Contains(text, "strings.split") {
		score += 1
		findings = append(findings, "Legacy string manipulation — evaluate strings.Cut")
	}

	// AST enrichment
	if traceMap != nil {
		if deadCode, ok := traceMap["dead_code"].(string); ok && deadCode != "" {
			score += 1
			findings = append(findings, "AST: dead code detected — modernization cleanup opportunity")
		}
	}

	return dialecticPillar(pillarNames[1], score, findings, "No significant modernization opportunities identified.")
}

// --- Pillar 3: Modularization ---

func (e *Engine) thesisModularization(
	ctx context.Context, text string, traceMap map[string]interface{}, isOrchestrator bool,
) models.DialecticPillar {
	score := 5
	var findings []string

	// Mixed-concern imports in a single file
	importDomains := 0
	if strings.Contains(text, "http") || strings.Contains(text, "net/") {
		importDomains++
	}
	if strings.Contains(text, "database") || strings.Contains(text, "sql") {
		importDomains++
	}
	if strings.Contains(text, "crypto") || strings.Contains(text, "auth") {
		importDomains++
	}
	if importDomains >= 2 {
		score += 2
		findings = append(findings, "Mixed concerns detected — consider package decomposition")
	}

	if strings.Contains(text, "god") || strings.Contains(text, "monolith") {
		score += 1
		findings = append(findings, "Monolithic structure indicators — modularization beneficial")
	}

	// AST enrichment: function density
	if traceMap != nil {
		if funcs, ok := traceMap["total_functions"].(float64); ok && funcs > 20 {
			score += 2
			findings = append(findings, fmt.Sprintf("AST: %d functions in package — high density suggests decomposition", int(funcs)))
		}
		if deps, ok := traceMap["dependencies"].(string); ok && strings.Contains(deps, "circular") {
			score += 1
			findings = append(findings, "AST: circular dependency indicators detected")
		}
	}

	// Orchestrator: delegate to EvaluateQualityAttributes for Modularity score
	if isOrchestrator && traceMap != nil {
		metrics, err := e.EvaluateQualityAttributes(ctx, text, traceMap)
		if err == nil {
			for _, m := range metrics {
				if m.Attribute == "Modularity" && m.Score < 5 {
					score += 2
					findings = append(findings, fmt.Sprintf("Engine: Modularity quality score %d/10 — improvement needed", m.Score))
				}
			}
		}
	}

	return dialecticPillar(pillarNames[2], score, findings, "No significant modularization opportunities identified.")
}

// --- Pillar 4: Efficiency ---

func (e *Engine) thesisEfficiency(
	text string, traceMap map[string]interface{},
) models.DialecticPillar {
	score := 5
	var findings []string

	if strings.Contains(text, "append") && !strings.Contains(text, "make(") {
		score += 1
		findings = append(findings, "append without pre-allocation — risk of excessive GC pressure")
	}
	if strings.Contains(text, "reflect") {
		score += 2
		findings = append(findings, "Reflection in logic paths — significant performance overhead")
	}
	if strings.Contains(text, "fmt.sprintf") && strings.Contains(text, "loop") {
		score += 1
		findings = append(findings, "fmt.Sprintf in loop — consider strings.Builder")
	}
	if strings.Contains(text, "string") && strings.Contains(text, "+=") {
		score += 1
		findings = append(findings, "String concatenation pattern — pre-allocated Builder recommended")
	}

	// AST enrichment
	if traceMap != nil {
		if complexity, ok := traceMap["complexity"].([]interface{}); ok {
			for _, c := range complexity {
				if cv, ok := c.(float64); ok && cv > 15 {
					score += 1
					findings = append(findings, "AST: cyclomatic complexity > 15 — hot-path optimization candidates")
					break
				}
			}
		}
		if conc, ok := traceMap["concurrency"].(string); ok && strings.Contains(conc, "goroutine") {
			score += 1
			findings = append(findings, "AST: goroutine patterns detected — efficiency review warranted")
		}
	}

	return dialecticPillar(pillarNames[3], score, findings, "No significant efficiency improvements identified.")
}

// --- Pillar 5: Reliability ---

func (e *Engine) thesisReliability(
	text string, traceMap map[string]interface{},
) models.DialecticPillar {
	score := 5
	var findings []string

	if !strings.Contains(text, "context") {
		score += 2
		findings = append(findings, "No context.Context references — propagation improvements needed")
	}
	if strings.Contains(text, "error") && !strings.Contains(text, "%w") {
		score += 1
		findings = append(findings, "Error handling without wrapping — add %%w for chain tracing")
	}
	if !strings.Contains(text, "test") {
		score += 1
		findings = append(findings, "No test references — testing coverage improvements recommended")
	}

	// AST enrichment
	if traceMap != nil {
		if coverage, ok := traceMap["coverage"].(float64); ok && coverage < 50 {
			score += 2
			findings = append(findings, fmt.Sprintf("AST: test coverage %.0f%% — below threshold", coverage))
		}
		if conc, ok := traceMap["concurrency"].(string); ok && strings.Contains(conc, "leak") {
			score += 1
			findings = append(findings, "AST: potential goroutine leak patterns detected")
		}
		if errPat, ok := traceMap["error_patterns"].(string); ok && strings.Contains(errPat, "unwrapped") {
			score += 1
			findings = append(findings, "AST: unwrapped error patterns confirmed")
		}
	}

	return dialecticPillar(pillarNames[4], score, findings, "No significant reliability improvements identified.")
}

// --- Pillar 6: Maintainability ---

func (e *Engine) thesisMaintainability(
	text string, traceMap map[string]interface{},
) models.DialecticPillar {
	score := 5
	var findings []string

	if strings.Contains(text, "todo") || strings.Contains(text, "fixme") || strings.Contains(text, "hack") {
		score += 1
		findings = append(findings, "TODO/FIXME/HACK markers — unresolved maintenance debt")
	}
	if strings.Contains(text, "deprecated") {
		score += 1
		findings = append(findings, "Deprecated code references — cleanup opportunity")
	}
	if !strings.Contains(text, "//") && !strings.Contains(text, "doc") {
		score += 1
		findings = append(findings, "Minimal documentation — readability improvements recommended")
	}

	// AST enrichment
	if traceMap != nil {
		if deadCode, ok := traceMap["dead_code"].(string); ok && deadCode != "" {
			score += 2
			findings = append(findings, "AST: dead code detected — removal improves maintainability")
		}
		if nodes, ok := traceMap["total_nodes"].(float64); ok {
			if funcs, ok := traceMap["total_functions"].(float64); ok && funcs > 0 {
				ratio := nodes / funcs
				if ratio > 50 {
					score += 1
					findings = append(findings, "AST: high node-to-function density — consider decomposition")
				}
			}
		}
		if cyclo, ok := traceMap["cyclomatic"].(float64); ok && cyclo > 10 {
			score += 1
			findings = append(findings, fmt.Sprintf("AST: average cyclomatic complexity %.0f — refactoring recommended", cyclo))
		}
	}

	return dialecticPillar(pillarNames[5], score, findings, "No significant maintainability improvements identified.")
}

// --- Shared helpers ---

func dialecticPillar(name string, score int, findings []string, defaultFinding string) models.DialecticPillar {
	if score > 10 {
		score = 10
	}
	if score < 1 {
		score = 1
	}
	finding := defaultFinding
	if len(findings) > 0 {
		finding = strings.Join(findings, "; ")
	}
	return models.DialecticPillar{Name: name, Score: score, Finding: finding}
}

func computeThesisVerdict(pillars []models.DialecticPillar) string {
	if len(pillars) == 0 {
		return "REVIEW"
	}
	total := 0
	for _, p := range pillars {
		total += p.Score
	}
	avg := total / len(pillars)
	switch {
	case avg >= 7:
		return "APPROVE"
	case avg >= 4:
		return "REVIEW"
	default:
		return "REJECT"
	}
}

func buildThesisNarrative(pillars []models.DialecticPillar, standards string) string {
	var b strings.Builder
	b.WriteString("## Thesis: Codebase Modernization Analysis\n\n")
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
