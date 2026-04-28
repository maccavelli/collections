package engine

import (
	"context"
	"fmt"
	"strings"

	"mcp-server-brainstorm/internal/models"
)

// forkRegistry defines triggers for architectural components.
var forkRegistry = []models.DecisionFork{
	{
		Component:      "Database",
		SocraticPrompt: "What is your primary requirement for data consistency?",
		Options: map[string]string{
			"Strict":   "Full ACID compliance. Best for transactions (Postgres, MySQL).",
			"Eventual": "High availability, lower consistency. Best for massive scale (NoSQL/DynamoDB).",
			"Document": "Flexible schema for rapid iteration (MongoDB).",
		},
		Impact:         "Consistency vs. Scalability trade-off.",
		Recommendation: "Start with Strict (Postgres) unless you expect >1M concurrent users.",
	},
	{
		Component:      "Queue",
		SocraticPrompt: "Is message ordering critical for your domain?",
		Options: map[string]string{
			"FIFO":      "Strict First-In-First-Out ordering (Amazon SQS FIFO, Kafka Partition).",
			"Standard":  "Best-effort ordering, higher throughput (Standard SQS).",
			"Real-time": "In-memory message passing for sub-ms latency (Redis Pub/Sub).",
		},
		Impact:         "Operational complexity vs. Correctness.",
		Recommendation: "Use Standard unless you are processing financial transactions.",
	},
	{
		Component:      "Auth",
		SocraticPrompt: "How do you plan to manage user sessions and identities?",
		Options: map[string]string{
			"JWT":     "Stateless tokens. Scale-friendly but hard to revoke (Auth0, Clerk).",
			"OIDC":    "Full external identity provider (Google/GitHub login).",
			"Session": "Stateful database-backed sessions. Secure and easy to revoke.",
		},
		Impact:         "Security surface and Implementation speed.",
		Recommendation: "Use OIDC (Google/GitHub) to avoid managing passwords yourself.",
	},
	{
		Component:      "API",
		SocraticPrompt: "How will your services communicate with each other?",
		Options: map[string]string{
			"REST":    "Standard HTTP/JSON. Ubiquitous and easy to debug.",
			"gRPC":    "High-performance binary protocol. Best for internal microservices.",
			"GraphQL": "Flexible client-side querying. Best for complex frontends.",
		},
		Impact:         "Developer productivity vs. Network efficiency.",
		Recommendation: "Start with REST; it has the best ecosystem support.",
	},
}

// GetDecisionForks identifies architectural forks based on
// requirement triggers.
func (e *Engine) GetDecisionForks(
	ctx context.Context, requirements string,
) ([]models.DecisionFork, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	lower := strings.ToLower(requirements)
	var forks []models.DecisionFork

	for _, f := range forkRegistry {
		trigger := strings.ToLower(f.Component)
		if strings.Contains(lower, trigger) {
			// Basic filtering: if the requirement already
			// contains an option's keyword, skip the fork
			// or mark as resolved.
			alreadyDefined := false
			for optKey := range f.Options {
				if strings.Contains(lower, strings.ToLower(optKey)) {
					alreadyDefined = true
					break
				}
			}

			if !alreadyDefined {
				forks = append(forks, f)
			}
		}
	}

	return forks, nil
}

// ClarifyRequirements analyzes input and returns
// structured forks and a narrative summary.
func (e *Engine) ClarifyRequirements(
	ctx context.Context, requirements string, standards string,
) (models.ClarificationResponse, error) {
	forks, err := e.GetDecisionForks(ctx, requirements)
	if err != nil {
		return models.ClarificationResponse{}, err
	}

	narrative := "Requirement analysis complete."
	if len(forks) > 0 {
		narrative = fmt.Sprintf(
			"Detected %d architectural decision points.",
			len(forks),
		)
	} else {
		narrative = "Requirements look grounded. No major forks detected."
	}

	var sb strings.Builder
	sb.WriteString("### Requirement Grounding Analysis\n\n")

	if len(forks) > 0 {
		for _, f := range forks {
			sb.WriteString(fmt.Sprintf("#### %s: %s\n", f.Component, f.SocraticPrompt))
			sb.WriteString("- **Options**:\n")
			for k, v := range f.Options {
				sb.WriteString(fmt.Sprintf("  - **%s**: %s\n", k, v))
			}
			sb.WriteString(fmt.Sprintf("- **Impact**: %s\n", f.Impact))
			sb.WriteString(fmt.Sprintf("- **Recommendation**: %s\n\n", f.Recommendation))
		}
	} else {
		sb.WriteString("The current requirements are specific enough for a baseline implementation. Consider defining performance targets next.\n")
	}

	if standards != "" {
		sb.WriteString("\n### Enterprise Standards Alignment\n")
		sb.WriteString(standards + "\n\n")
		narrative += " (Anchored by Enterprise Standards)"
	}

	return models.ClarificationResponse{
		Summary: narrative,
		Data: struct {
			Narrative string                `json:"narrative"`
			Forks     []models.DecisionFork `json:"forks"`
		}{
			Narrative: narrative,
			Forks:     forks,
		},
	}, nil
}
