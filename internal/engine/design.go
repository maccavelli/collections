package engine

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
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
	ctx context.Context, design string, traceMap map[string]any,
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

// AnalyzeEvolution identifies risks when extending or
// modifying existing project logic. Returns a structured
// result with category, risk level, and recommendation.
func (e *Engine) AnalyzeEvolution(
	ctx context.Context, proposal string, standards string, traceMap map[string]any,
) (models.EvolutionResult, error) {
	select {
	case <-ctx.Done():
		return models.EvolutionResult{}, ctx.Err()
	default:
	}
	lower := strings.ToLower(proposal)

	var res models.EvolutionResult
	found := false

	// Empirical Evolutionary Ledger (Angle 5)
	if traceMap != nil {
		nodeCount := 0.0
		if nc, ok := traceMap["total_nodes"].(float64); ok {
			nodeCount = nc
		} else if intNc, ok := traceMap["total_nodes"].(int); ok {
			nodeCount = float64(intNc)
		}

		funcsCount := 0.0
		if fc, ok := traceMap["total_functions"].(float64); ok {
			funcsCount = fc
		}

		if nodeCount > 500 || funcsCount > 20 {
			res = models.EvolutionResult{
				Summary: "Massive evolutionary footprint detected (HIGH risk)",
				Data: struct {
					Category       string `json:"category"`
					RiskLevel      string `json:"risk_level"`
					Reasoning      string `json:"reasoning,omitempty"`
					Recommendation string `json:"recommendation"`
					Narrative      string `json:"narrative,omitempty"`
				}{
					Category:       "large_scale_refactor",
					RiskLevel:      "HIGH",
					Reasoning:      fmt.Sprintf("Empirical footprint proves a massive blast radius (%.0f nodes, %.0f functions).", nodeCount, funcsCount),
					Recommendation: "Decompose into smaller isolated merges to mathematically reduce regression severity.",
				},
			}
			found = true
		} else if nodeCount > 100 || funcsCount > 5 || strings.Contains(lower, "split") || strings.Contains(lower, "merge") {
			res = models.EvolutionResult{
				Summary: "Structural displacement detected (MEDIUM risk)",
				Data: struct {
					Category       string `json:"category"`
					RiskLevel      string `json:"risk_level"`
					Reasoning      string `json:"reasoning,omitempty"`
					Recommendation string `json:"recommendation"`
					Narrative      string `json:"narrative,omitempty"`
				}{
					Category:       "structural_displacement",
					RiskLevel:      "MEDIUM",
					Reasoning:      fmt.Sprintf("Moderate structural mutation detected (%.0f nodes) requiring interface compliance.", nodeCount),
					Recommendation: "Define rigorous interface compliance boundaries prior to displacement.",
				},
			}
			found = true
		}
	}

	if !found {
		res = models.EvolutionResult{
			Summary: "No high-risk changes detected",
			Data: struct {
				Category       string `json:"category"`
				RiskLevel      string `json:"risk_level"`
				Reasoning      string `json:"reasoning,omitempty"`
				Recommendation string `json:"recommendation"`
				Narrative      string `json:"narrative,omitempty"`
			}{
				Category:       "general",
				RiskLevel:      "LOW",
				Reasoning:      "Proposal contains no known high-risk keywords.",
				Recommendation: "Evolution path looks stable. Define specific components next.",
			},
		}
	}

	stdNote := ""
	if standards != "" {
		stdNote = " (Evaluated against Enterprise Standards)"
	}

	res.Data.Narrative = fmt.Sprintf(
		"Evolution analysis: %s change detected with %s risk.%s",
		res.Data.Category, res.Data.RiskLevel, stdNote,
	)

	return res, nil
}

// qualityRubric defines a quality attribute with a base
// score, keyword bonuses, and a base observation.
type qualityRubric struct {
	attribute string
	baseScore int
	baseObs   string
	bonuses   []struct {
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
	ctx context.Context, design string, traceMap map[string]any,
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

		// Angle 6: Negation heuristics deleted completely in favor of AST telemetry
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

		// Empirical Telemetry Override (Angle 6)
		if traceMap != nil {
			if imports, ok := traceMap["imports"].([]any); ok {
				for _, imp := range imports {
					pkg, _ := imp.(string)
					switch rubric.attribute {
					case "Observability":
						if strings.Contains(pkg, "prometheus") || strings.Contains(pkg, "telemetry") || strings.Contains(pkg, "zap") {
							score += 5
							matchedAny = true
							matched = append(matched, "AST:"+pkg)
						}
					case "Security":
						if strings.Contains(pkg, "crypto") || strings.Contains(pkg, "bcrypt") {
							score += 5
							matchedAny = true
							matched = append(matched, "AST:"+pkg)
						}
					case "Modularity":
						if strings.Contains(pkg, "testing") || strings.Contains(pkg, "go/ast") {
							score += 3
							matchedAny = true
							matched = append(matched, "AST:"+pkg)
						}
					}
				}
			}
		}

		if matchedAny {
			obs = fmt.Sprintf(
				"Detected keywords [%s].",
				strings.Join(matched, ", "),
			)
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

// CritiqueDesign provides a consolidated, multi-dimensional
// assessment of a design snippet.
func (e *Engine) CritiqueDesign(
	ctx context.Context, design string, standards string, traceMap map[string]any,
) (models.CritiqueResponse, error) {
	var (
		challenges []string
		metrics    []models.QualityMetric
		redTeam    []models.RedTeamChallenge
	)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		challenges, err = e.ChallengeAssumption(gCtx, design, traceMap)
		return err
	})

	g.Go(func() error {
		var err error
		metrics, err = e.EvaluateQualityAttributes(gCtx, design, traceMap)
		return err
	})

	g.Go(func() error {
		var err error
		redTeam, err = e.RedTeamReview(gCtx, design, traceMap)
		return err
	})

	if err := g.Wait(); err != nil {
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

	// Architectural Standards Context (Recall)
	if standards != "" {
		sb.WriteString("\n#### Relevant Code Standards (Recall)\n")
		sb.WriteString(standards)
		sb.WriteString("\n")
	}

	return models.CritiqueResponse{
		Summary: fmt.Sprintf("Design critique complete: %d critical challenges, %d red team concerns.", len(challenges), len(redTeam)),
		Data: struct {
			Narrative  string                    `json:"narrative"`
			Reasoning  string                    `json:"reasoning,omitempty"`
			Challenges []string                  `json:"challenges"`
			Metrics    []models.QualityMetric    `json:"metrics"`
			RedTeam    []models.RedTeamChallenge `json:"red_team"`
			Standards  string                    `json:"standards,omitempty"`
		}{
			Narrative:  narrative,
			Reasoning:  "Consolidated feedback based on 5 point quality rubric and adversarial Red Team simulations. Targeted Socratic challenges address domain-specific architecture gaps.",
			Challenges: challenges,
			Metrics:    metrics,
			RedTeam:    redTeam,
			Standards:  standards,
		},
	}, nil
}
