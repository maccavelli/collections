package engine

import (
	"context"
	"fmt"
	"strings"

	"mcp-server-brainstorm/internal/models"
)

// challengeKeywords maps domain categories to their
// challenge questions for ChallengeAssumption.
var challengeKeywords = []struct {
	keywords   []string
	challenges []string
}{
	{
		keywords: []string{"db", "database", "sql",
			"postgres", "mysql", "mongo"},
		challenges: []string{
			"What is the retry strategy if the" +
				" database connection drops" +
				" mid-operation?",
			"How do you handle schema migrations" +
				" without downtime?",
		},
	},
	{
		keywords: []string{"api", "http", "rest",
			"endpoint", "grpc"},
		challenges: []string{
			"How do you handle authentication" +
				" timeouts or invalid tokens?",
			"What is the rate-limiting strategy" +
				" for this endpoint?",
		},
	},
	{
		keywords: []string{"queue", "async", "event",
			"pubsub", "kafka", "rabbitmq"},
		challenges: []string{
			"What happens to in-flight messages" +
				" if the consumer crashes?",
		},
	},
	{
		keywords: []string{"cache", "redis",
			"memcache"},
		challenges: []string{
			"What is the cache invalidation" +
				" strategy? How do you handle" +
				" stale data?",
		},
	},
	{
		keywords: []string{"deploy", "kubernetes",
			"container", "docker"},
		challenges: []string{
			"What is the rollback plan if" +
				" deployment fails mid-update?",
		},
	},
	{
		keywords: []string{"config", "env",
			"environment", "secret"},
		challenges: []string{
			"How are secrets managed and rotated?" +
				" Are they ever logged?",
		},
	},
	{
		keywords: []string{"state", "session",
			"persist"},
		challenges: []string{
			"What happens to in-memory state if" +
				" the process restarts?",
		},
	},
}

// generalChallenges are fallback questions appended when
// fewer than 2 domain-specific challenges are found.
var generalChallenges = []string{
	"How does this handle high latency from" +
		" external dependencies?",
	"What happens if the input payload" +
		" exceeds 10MB?",
	"How would you roll back this change if" +
		" it fails in production?",
}

// ChallengeAssumption performs a stress test on a design
// snippet and returns targeted challenge questions. Only
// returns domain-matched challenges; falls back to a
// single general challenge when nothing matches.
func (e *Engine) ChallengeAssumption(
	ctx context.Context, design string,
) ([]string, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	lower := strings.ToLower(design)
	var challenges []string

	for _, group := range challengeKeywords {
		for _, kw := range group.keywords {
			if strings.Contains(lower, kw) {
				challenges = append(
					challenges, group.challenges...,
				)
				break
			}
		}
	}

	// Single general fallback when no domain matched.
	if len(challenges) == 0 {
		challenges = append(
			challenges, generalChallenges[0],
		)
	}

	return challenges, nil
}

type evolutionPattern struct {
	keywords []string
	result   models.EvolutionResult
}

// evolutionPatterns maps keywords to structured evolution
// analysis results.
var evolutionPatterns = []evolutionPattern{
	{
		keywords: []string{"refactor"},
		result: models.EvolutionResult{
			Category:  "refactor",
			RiskLevel: "HIGH",
			Reasoning: "Refactoring often involves heavy logic " +
				"displacement with high regression risk.",
			Recommendation: "Identify all upstream" +
				" dependencies before proceeding." +
				" Ensure existing tests cover" +
				" current behavior.",
		},
	},
	{
		keywords: []string{"deprecat"},
		result: models.EvolutionResult{
			Category:  "deprecation",
			RiskLevel: "HIGH",
			Reasoning: "Removing established APIs can " +
				"break downstream consumers " +
				"permanently.",
			Recommendation: "Ensure backward" +
				" compatibility or provide a" +
				" migration guide.",
		},
	},
	{
		keywords: []string{"rename"},
		result: models.EvolutionResult{
			Category:  "rename",
			RiskLevel: "MEDIUM",
			Reasoning: "Naming changes require broad " +
				"re-factoring and can cause build " +
				"failures if missed.",
			Recommendation: "Update all references." +
				" Consider aliasing the old name" +
				" during transition.",
		},
	},
	{
		keywords: []string{"split", "extract"},
		result: models.EvolutionResult{
			Category:  "split",
			RiskLevel: "MEDIUM",
			Reasoning: "Breaking down components " +
				"introduces communication overhead " +
				"and interface debt.",
			Recommendation: "Define clear interfaces" +
				" between the split components." +
				" Verify no circular dependencies.",
		},
	},
	{
		keywords: []string{"merge", "consolidat"},
		result: models.EvolutionResult{
			Category:  "merge",
			RiskLevel: "MEDIUM",
			Reasoning: "Consolidating logic can " +
				"obscure individual responsibilities " +
				"and create monoliths.",
			Recommendation: "Reconcile divergent" +
				" behaviors and configurations." +
				" Test combined edge cases.",
		},
	},
	{
		keywords: []string{"rewrite"},
		result: models.EvolutionResult{
			Category:  "rewrite",
			RiskLevel: "HIGH",
			Reasoning: "Full rewrites often ignore " +
				"legacy bugfixes and critical edge " +
				"cases.",
			Recommendation: "Consider incremental" +
				" replacement over full rewrite." +
				" Maintain a compatibility layer.",
		},
	},
	{
		keywords: []string{"dependency", "upgrade",
			"update"},
		result: models.EvolutionResult{
			Category:  "dependency_change",
			RiskLevel: "MEDIUM",
			Reasoning: "Third-party updates can " +
				"introduce silent regressions or " +
				"security vulnerabilities.",
			Recommendation: "Check changelogs for" +
				" breaking changes. Pin and test" +
				" the exact version.",
		},
	},
	{
		keywords: []string{"remove", "delete", "drop"},
		result: models.EvolutionResult{
			Category:  "removal",
			RiskLevel: "HIGH",
			Reasoning: "Deletions are irreversible " +
				"in runtime and might orphan " +
				"critical dependencies.",
			Recommendation: "Verify no downstream" +
				" consumers depend on the removed" +
				" component.",
		},
	},
	{
		keywords: []string{"add", "new", "implement"},
		result: models.EvolutionResult{
			Category:  "addition",
			RiskLevel: "LOW",
			Reasoning: "New additions have lower " +
				"impact on existing paths but " +
				"increase maintenance surface.",
			Recommendation: "Define acceptance" +
				" criteria. Ensure new component" +
				" follows existing conventions.",
		},
	},
	{
		keywords: []string{"replace", "swap",
			"substitut"},
		result: models.EvolutionResult{
			Category:  "replacement",
			RiskLevel: "MEDIUM",
			Reasoning: "Swapping implementations " +
				"requires strict behavioral parity " +
				"alignment.",
			Recommendation: "Ensure the replacement" +
				" covers all use cases of the" +
				" original. Run comparative tests.",
		},
	},
}

// AnalyzeEvolution identifies risks when extending or
// modifying existing project logic. Returns a structured
// result with category, risk level, and recommendation.
func (e *Engine) AnalyzeEvolution(
	ctx context.Context, proposal string,
) (models.EvolutionResult, error) {
	select {
	case <-ctx.Done():
		return models.EvolutionResult{}, ctx.Err()
	default:
	}
	lower := strings.ToLower(proposal)

	var res models.EvolutionResult
	found := false
	for _, pat := range evolutionPatterns {
		for _, kw := range pat.keywords {
			if strings.Contains(lower, kw) {
				res = pat.result
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		res = models.EvolutionResult{
			Category:  "general",
			RiskLevel: "LOW",
			Reasoning: "Proposal contains no known high-risk keywords.",
			Recommendation: "Evolution path looks stable." +
				" Define specific components next.",
		}
	}

	res.Narrative = fmt.Sprintf(
		"Evolution analysis: %s change detected with %s risk.",
		res.Category, res.RiskLevel,
	)

	var sb strings.Builder
	sb.WriteString("### Evolution Analysis\n\n")
	sb.WriteString(fmt.Sprintf("- **Category**: %s\n", res.Category))
	sb.WriteString(fmt.Sprintf("- **Risk Level**: %s\n", res.RiskLevel))
	sb.WriteString(fmt.Sprintf("- **Recommendation**: %s\n", res.Recommendation))
	res.SummaryMD = sb.String()

	return res, nil
}

// qualityRubric defines a quality attribute with a base
// score, keyword bonuses, and a base observation.
type qualityRubric struct {
	attribute   string
	baseScore   int
	baseObs     string
	bonuses     []struct {
		keyword string
		bonus   int
	}
	negPatterns []string
}

// qualityRubrics defines the scoring system for quality
// attributes. Scores start at the base and add bonuses
// for each matched keyword, capped at 10.
var qualityRubrics = []qualityRubric{
	{
		attribute: "Scalability",
		baseScore: 4,
		baseObs:   "No scaling strategy mentioned.",
		bonuses: []struct {
			keyword string
			bonus   int
		}{
			{"cache", 2},
			{"redis", 2},
			{"horizontal", 3},
			{"replicas", 3},
			{"shard", 2},
			{"partition", 2},
			{"load balanc", 2},
		},
	},
	{
		attribute: "Security",
		baseScore: 3,
		baseObs:   "No auth or encryption mentioned.",
		bonuses: []struct {
			keyword string
			bonus   int
		}{
			{"auth", 2},
			{"token", 2},
			{"encrypt", 2},
			{"tls", 2},
			{"rbac", 2},
			{"firewall", 1},
			{"secret", 1},
		},
		negPatterns: []string{
			"no auth", "without auth",
			"no encrypt", "without encrypt",
		},
	},
	{
		attribute: "Observability",
		baseScore: 3,
		baseObs: "No logging or monitoring" +
			" mentioned.",
		bonuses: []struct {
			keyword string
			bonus   int
		}{
			{"log", 2},
			{"monitor", 2},
			{"metric", 2},
			{"trace", 2},
			{"alert", 1},
			{"dashboard", 1},
		},
	},
	{
		attribute: "Modularity",
		baseScore: 5,
		baseObs:   "Modularity appears adequate.",
		bonuses: []struct {
			keyword string
			bonus   int
		}{
			{"interface", 2},
			{"plugin", 2},
			{"modular", 2},
			{"microservice", 2},
			{"decouple", 1},
			{"abstract", 1},
		},
	},
	{
		attribute: "Performance",
		baseScore: 4,
		baseObs:   "No specific performance optimizations noted.",
		bonuses: []struct {
			keyword string
			bonus   int
		}{
			{"latency", 2},
			{"throughput", 2},
			{"buffer", 1},
			{"pool", 2},
			{"index", 2},
			{"cache", 1},
			{"concurrent", 2},
		},
	},
}

// EvaluateQualityAttributes audits the design against
// quality rubrics and returns scored metrics using
// additive keyword matching. Each matched keyword adds
// its bonus to the base score, capped at 10.
func (e *Engine) EvaluateQualityAttributes(
	ctx context.Context, design string,
) ([]models.QualityMetric, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	lower := strings.ToLower(design)
	var metrics []models.QualityMetric

	for _, rubric := range qualityRubrics {
		score := rubric.baseScore
		obs := rubric.baseObs
		matchedAny := false

		// Check for negation patterns first.
		negated := false
		for _, neg := range rubric.negPatterns {
			if strings.Contains(lower, neg) {
				negated = true
				break
			}
		}

		if !negated {
			var matched []string
			for _, b := range rubric.bonuses {
				if strings.Contains(lower, b.keyword) {
					score += b.bonus
					matched = append(
						matched, b.keyword,
					)
					matchedAny = true
				}
			}
			if matchedAny {
				obs = fmt.Sprintf(
					"Detected keywords [%s].",
					strings.Join(matched, ", "),
				)
			}
		} else {
			obs = "Negation detected —" +
				" explicit absence noted."
		}

		// Cap at 10.
		if score > 10 {
			score = 10
		}

		metrics = append(metrics, models.QualityMetric{
			Attribute:   rubric.attribute,
			Score:       score,
			Observation: obs,
		})
	}

	return metrics, nil
}

// RedTeamReview simulates adversarial personas to
// challenge the design from multiple angles. Returns
// compact structured challenges.
func (e *Engine) RedTeamReview(
	ctx context.Context, design string,
) ([]models.RedTeamChallenge, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	lower := strings.ToLower(design)
	var out []models.RedTeamChallenge

	// Run all persona checks
	out = append(out, e.checkMaintenancePersona(lower)...)
	out = append(out, e.checkSecurityPersona(lower)...)
	out = append(out, e.checkScalabilityPersona(lower)...)
	out = append(out, e.checkCompatibilityPersona(lower)...)
	out = append(out, e.checkReliabilityPersona(lower)...)
	out = append(out, e.checkPerformancePersona(lower)...)
	out = append(out, e.checkCompliancePersona(lower)...)
	out = append(out, e.checkOperationsPersona(lower)...)

	// Fallback if no issues found
	if len(out) == 0 {
		out = append(out,
			models.RedTeamChallenge{
				Persona:  "Maintenance",
				Question: "Where is the documentation?",
			},
			models.RedTeamChallenge{
				Persona:  "Scalability",
				Question: "What if load triples?",
			},
		)
	}

	return out, nil
}

func (e *Engine) checkMaintenancePersona(design string) []models.RedTeamChallenge {
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

func (e *Engine) checkSecurityPersona(design string) []models.RedTeamChallenge {
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

func (e *Engine) checkScalabilityPersona(design string) []models.RedTeamChallenge {
	if strings.Contains(design, "single") ||
		strings.Contains(design, "monolith") ||
		!strings.Contains(design, "scale") {
		return []models.RedTeamChallenge{{
			Persona:  "Scalability",
			Question: "What if load triples? Where is the bottleneck?",
		}}
	}
	return nil
}

func (e *Engine) checkCompatibilityPersona(design string) []models.RedTeamChallenge {
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

func (e *Engine) checkReliabilityPersona(design string) []models.RedTeamChallenge {
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

func (e *Engine) checkPerformancePersona(design string) []models.RedTeamChallenge {
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

func (e *Engine) checkCompliancePersona(design string) []models.RedTeamChallenge {
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

func (e *Engine) checkOperationsPersona(design string) []models.RedTeamChallenge {
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


// CritiqueDesign provides a consolidated, multi-dimensional
// assessment of a design snippet.
func (e *Engine) CritiqueDesign(
	ctx context.Context, design string,
) (models.CritiqueResponse, error) {
	challenges, err := e.ChallengeAssumption(ctx, design)
	if err != nil {
		return models.CritiqueResponse{}, err
	}
	metrics, err := e.EvaluateQualityAttributes(ctx, design)
	if err != nil {
		return models.CritiqueResponse{}, err
	}
	redTeam, err := e.RedTeamReview(ctx, design)
	if err != nil {
		return models.CritiqueResponse{}, err
	}

	narrative := "Design critique complete. "
	if len(redTeam) > 0 {
		narrative += fmt.Sprintf(
			"Red team raised %d concerns.",
			len(redTeam),
		)
	}

	var sb strings.Builder
	sb.WriteString("### Design Critique Summary\n\n")

	// Quality Metrics Table.
	sb.WriteString("#### Quality Attributes\n")
	sb.WriteString("| Attribute | Score | Observation |\n")
	sb.WriteString("| :--- | :---: | :--- |\n")
	for _, m := range metrics {
		sb.WriteString(fmt.Sprintf(
			"| %s | %d/10 | %s |\n",
			m.Attribute, m.Score, m.Observation,
		))
	}

	// Red Team Challenges.
	if len(redTeam) > 0 {
		sb.WriteString("\n#### Red Team Review\n")
		for _, r := range redTeam {
			sb.WriteString(fmt.Sprintf(
				"- **%s**: %s\n",
				r.Persona, r.Question,
			))
		}
	}

	// Socratic Challenges.
	if len(challenges) > 0 {
		sb.WriteString("\n#### Critical Challenges\n")
		for _, c := range challenges {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	return models.CritiqueResponse{
		Narrative:  narrative,
		Reasoning:  "Consolidated feedback based on 5 point quality rubric and adversarial Red Team simulations. Targeted Socratic challenges address domain-specific architecture gaps.",
		SummaryMD:  sb.String(),
		Challenges: challenges,
		Metrics:    metrics,
		RedTeam:    redTeam,
	}, nil
}
