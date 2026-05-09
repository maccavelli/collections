// Package handler provides MCP tool handlers for the MagicDev pipeline.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/integration"
	"mcp-server-magicdev/internal/integration/llm"
)

// runAporiaEngine executes the server-side Aporia Engine logic.
// It augments the agent-provided SynthesisResolution with:
// 1. BuntDB-validated constraint filtering (self-healing)
// 2. Tri-factor decision matrix evaluation
// 3. LLM-enhanced synthesis (auto-enabled when configured)
//
// If ChaosAnalysis is nil, the engine skips all Chaos processing and
// returns the agent-provided synthesis unmodified (backward compat).
func runAporiaEngine(
	ctx context.Context,
	store *db.Store,
	session *db.SessionState,
	thesis *db.DesignProposal,
	antithesis *db.SkepticAnalysis,
	chaos *db.ChaosAnalysis,
	agentSynthesis *db.SynthesisResolution,
) *db.SynthesisResolution {
	// Start with agent-provided synthesis as baseline
	synthesis := agentSynthesis
	if synthesis == nil {
		synthesis = &db.SynthesisResolution{}
	}

	// If no Chaos input, return agent synthesis as-is (backward compat)
	if chaos == nil {
		slog.Debug("aporia_engine: no chaos analysis provided, using agent synthesis")
		return synthesis
	}

	// Step 1: Constraint Filtering against BuntDB standards
	filterConstraints(store, session, chaos, synthesis)

	// Step 2: Decision Matrix — evaluate fatal flaws
	evaluateDecisionMatrix(chaos, synthesis)

	// Step 3: LLM Enhancement (auto-enabled when configured)
	llmClient, err := integration.NewLLMClient(store)
	if err == nil && llmClient != nil {
		enhanced, err := llmEnhanceSynthesis(ctx, llmClient, session, thesis, antithesis, chaos, synthesis)
		if err == nil {
			synthesis = enhanced
			synthesis.LLMEnhanced = true
			slog.Info("aporia_engine: LLM synthesis applied successfully")
		} else {
			slog.Warn("aporia_engine: LLM synthesis failed at runtime, using deterministic fallback",
				"error", err,
			)
		}
	} else {
		slog.Debug("aporia_engine: LLM not configured, using deterministic synthesis")
	}

	// Import Chaos data into synthesis
	synthesis.ChaosVetted = len(chaos.FatalFlaws) == 0 || allFatalFlawsResolved(chaos, synthesis)
	synthesis.RejectedOptions = chaos.RejectedPatterns
	synthesis.ConstraintLocks = chaos.Constraints

	return synthesis
}

// filterConstraints checks each Chaos constraint against BuntDB standards.
// If a matching standard exists, the constraint is auto-resolved (self-healing).
func filterConstraints(
	store *db.Store,
	session *db.SessionState,
	chaos *db.ChaosAnalysis,
	synthesis *db.SynthesisResolution,
) {
	if len(chaos.Constraints) == 0 || len(session.Standards) == 0 {
		return
	}

	standardsText := strings.Join(session.Standards, "\n")

	for i, constraint := range chaos.Constraints {
		// Search standards for relevant content matching the constraint domain
		searchTerms := []string{
			constraint.Domain,
			constraint.Constraint,
			constraint.Platform,
		}

		for _, term := range searchTerms {
			if term == "" {
				continue
			}
			if strings.Contains(strings.ToLower(standardsText), strings.ToLower(term)) {
				chaos.Constraints[i].Enforced = true
				synthesis.Decisions = append(synthesis.Decisions, db.ArchitecturalDecision{
					Topic:     fmt.Sprintf("Chaos Constraint [%s]: %s", constraint.Domain, constraint.Platform),
					Decision:  fmt.Sprintf("Constraint auto-resolved via ingested standard: %s", constraint.Constraint),
					Rationale: constraint.Impact,
				})
				slog.Debug("aporia_engine: constraint self-healed via standard",
					"domain", constraint.Domain,
					"platform", constraint.Platform,
				)
				break
			}
		}
	}
}

// evaluateDecisionMatrix applies the tri-factor scoring logic.
// Fatal flaws that cannot be resolved trigger True Aporia (escalation to user).
func evaluateDecisionMatrix(
	chaos *db.ChaosAnalysis,
	synthesis *db.SynthesisResolution,
) {
	if len(chaos.FatalFlaws) == 0 {
		return
	}

	for _, flaw := range chaos.FatalFlaws {
		// Each unresolved fatal flaw becomes an outstanding question
		synthesis.OutstandingQuestions = append(synthesis.OutstandingQuestions, db.GranularQuestion{
			Topic:    fmt.Sprintf("Chaos Fatal Flaw [%s]", flaw.Category),
			Question: flaw.Description,
			Context:  fmt.Sprintf("The Chaos Architect identified a %s-severity flaw: %s", flaw.Severity, flaw.MitigationStrategy),
			Impact:   "Architecture may need fundamental redesign to address this operational risk",
		})
	}
}

// allFatalFlawsResolved checks if every fatal flaw has been addressed
// by a constraint filter or an existing decision.
func allFatalFlawsResolved(chaos *db.ChaosAnalysis, synthesis *db.SynthesisResolution) bool {
	if len(chaos.FatalFlaws) == 0 {
		return true
	}

	// Build a set of resolved topics
	resolved := make(map[string]bool)
	for _, d := range synthesis.Decisions {
		resolved[d.Topic] = true
	}

	for _, flaw := range chaos.FatalFlaws {
		topic := fmt.Sprintf("Chaos Constraint [%s]: %s", flaw.Category, "all")
		if !resolved[topic] {
			return false
		}
	}
	return false // Conservative: assume not all resolved
}

// llmEnhanceSynthesis sends the full Socratic context to the configured LLM
// for enriched synthesis generation. Returns the enhanced synthesis or error.
// Retries up to 3 times with exponential backoff to handle transient failures
// (timeouts, rate limits, network blips) before falling back to deterministic mode.
func llmEnhanceSynthesis(
	ctx context.Context,
	client *llm.Client,
	session *db.SessionState,
	thesis *db.DesignProposal,
	antithesis *db.SkepticAnalysis,
	chaos *db.ChaosAnalysis,
	baseSynthesis *db.SynthesisResolution,
) (*db.SynthesisResolution, error) {
	prompt := buildAporiaPrompt(session, thesis, antithesis, chaos, baseSynthesis)

	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Apply timeout per attempt to prevent hanging the MCP tool response
		llmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

		start := time.Now()
		var enhanced db.SynthesisResolution
		err := client.JSONResponse(llmCtx, prompt, &enhanced)
		elapsed := time.Since(start)
		cancel()

		slog.Info("aporia_engine: LLM call completed",
			"attempt", attempt,
			"duration_ms", elapsed.Milliseconds(),
			"error", err,
		)

		if err == nil {
			// Validate LLM output
			if valErr := validateLLMSynthesis(&enhanced); valErr != nil {
				lastErr = fmt.Errorf("LLM output validation failed (attempt %d): %w", attempt, valErr)
				slog.Warn("aporia_engine: LLM validation failed, retrying",
					"attempt", attempt,
					"error", valErr,
				)
			} else {
				// Merge strategy: LLM narrative replaces, LLM decisions append to agent decisions
				merged := &db.SynthesisResolution{
					Narrative:              enhanced.Narrative,
					Decisions:              append(baseSynthesis.Decisions, enhanced.Decisions...),
					OutstandingQuestions:    mergeQuestions(baseSynthesis.OutstandingQuestions, enhanced.OutstandingQuestions),
					UnresolvedDependencies: mergeStringSlices(baseSynthesis.UnresolvedDependencies, enhanced.UnresolvedDependencies),
				}
				return merged, nil
			}
		} else {
			lastErr = fmt.Errorf("LLM synthesis failed (attempt %d): %w", attempt, err)
			slog.Warn("aporia_engine: LLM call failed, retrying",
				"attempt", attempt,
				"max_retries", maxRetries,
				"error", err,
			)
		}

		// Exponential backoff: 2s, 4s (skip delay after final attempt)
		if attempt < maxRetries {
			backoff := time.Duration(1<<attempt) * time.Second // 2s, 4s
			slog.Debug("aporia_engine: backing off before retry",
				"backoff_seconds", backoff.Seconds(),
				"next_attempt", attempt+1,
			)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, fmt.Errorf("LLM synthesis cancelled during backoff: %w", ctx.Err())
			}
		}
	}

	return nil, fmt.Errorf("LLM synthesis failed after %d attempts: %w", maxRetries, lastErr)
}

// validateLLMSynthesis checks the LLM response for basic sanity.
func validateLLMSynthesis(s *db.SynthesisResolution) error {
	if s.Narrative == "" && len(s.Decisions) == 0 {
		return fmt.Errorf("empty synthesis: no narrative or decisions")
	}

	// Field length safety — reject oversized responses
	const maxFieldLen = 2000
	if len(s.Narrative) > maxFieldLen*4 {
		return fmt.Errorf("narrative exceeds maximum length (%d > %d)", len(s.Narrative), maxFieldLen*4)
	}
	for _, d := range s.Decisions {
		if len(d.Rationale) > maxFieldLen {
			return fmt.Errorf("decision rationale exceeds maximum length")
		}
	}

	return nil
}

// buildAporiaPrompt constructs the structured prompt for LLM synthesis.
// SECURITY: This function MUST NOT include vault secrets. Only session data and standards text.
func buildAporiaPrompt(
	session *db.SessionState,
	thesis *db.DesignProposal,
	antithesis *db.SkepticAnalysis,
	chaos *db.ChaosAnalysis,
	baseSynthesis *db.SynthesisResolution,
) string {
	var b strings.Builder

	b.WriteString("You are the Aporia Engine — the Supreme Court of an architectural design pipeline.\n\n")
	b.WriteString("You have received three inputs from the Socratic Trifecta plus a Chaos Architect stress test.\n")
	b.WriteString("Your job is to produce a final synthesis that resolves all conflicts.\n\n")
	b.WriteString("RULES:\n")
	b.WriteString("1. If Thesis is logically sound AND Chaos-approved, lock the Thesis.\n")
	b.WriteString("2. If Thesis fails Chaos but Antithesis passes, pivot to the Antithesis.\n")
	b.WriteString("3. If ALL paths are Chaos-rejected, synthesize a Third Way or escalate as an outstanding question.\n")
	b.WriteString("4. For each decision, explain WHY with specific operational reasoning.\n")
	b.WriteString("5. Reference specific Chaos constraints and fatal flaws in your rationale.\n\n")

	// Thesis
	if thesis != nil {
		b.WriteString("=== THESIS (Design Proposal) ===\n")
		thesisJSON, _ := json.Marshal(thesis)
		truncated := truncateString(string(thesisJSON), 4000)
		b.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", truncated))
	}

	// Antithesis
	if antithesis != nil {
		b.WriteString("=== ANTITHESIS (Skeptic Analysis) ===\n")
		antiJSON, _ := json.Marshal(antithesis)
		truncated := truncateString(string(antiJSON), 4000)
		b.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", truncated))
	}

	// Chaos
	b.WriteString("=== CHAOS ARCHITECT (Operational Stress Test) ===\n")
	chaosJSON, _ := json.Marshal(chaos)
	truncated := truncateString(string(chaosJSON), 4000)
	b.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", truncated))

	// Standards context (truncated for token budget)
	if len(session.Standards) > 0 {
		combined := strings.Join(session.Standards, "\n---\n")
		b.WriteString("=== INGESTED STANDARDS (context) ===\n")
		b.WriteString(fmt.Sprintf("```\n%s\n```\n\n", truncateString(combined, 4000)))
	}

	// Agent's baseline synthesis
	if baseSynthesis != nil && len(baseSynthesis.Decisions) > 0 {
		b.WriteString("=== AGENT BASELINE SYNTHESIS ===\n")
		baseJSON, _ := json.Marshal(baseSynthesis.Decisions)
		b.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", truncateString(string(baseJSON), 2000)))
	}

	b.WriteString("OUTPUT: Return strictly valid JSON matching this schema (no markdown wrapping):\n")
	b.WriteString(`{"narrative": "...", "decisions": [{"topic": "...", "decision": "...", "rationale": "..."}], "outstanding_questions": [{"topic": "...", "question": "...", "context": "...", "impact": "..."}]}`)
	b.WriteString("\n")

	return b.String()
}

// truncateString truncates a string to maxLen characters, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}

// mergeQuestions deduplicates outstanding questions by topic.
func mergeQuestions(a, b []db.GranularQuestion) []db.GranularQuestion {
	seen := make(map[string]bool)
	var merged []db.GranularQuestion
	for _, q := range a {
		key := q.Topic + "|" + q.Question
		if !seen[key] {
			seen[key] = true
			merged = append(merged, q)
		}
	}
	for _, q := range b {
		key := q.Topic + "|" + q.Question
		if !seen[key] {
			seen[key] = true
			merged = append(merged, q)
		}
	}
	return merged
}

// mergeStringSlices deduplicates string slices.
func mergeStringSlices(a, b []string) []string {
	seen := make(map[string]bool)
	var merged []string
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			merged = append(merged, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			merged = append(merged, s)
		}
	}
	return merged
}
