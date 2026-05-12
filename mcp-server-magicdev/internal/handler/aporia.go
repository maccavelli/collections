// Package handler provides MCP tool handlers for the MagicDev pipeline.
package handler

import (
	"context"
	"encoding/json"
	"errors"
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

	// 1. If Chaos is missing or entirely sparse, generate Deterministic Chaos
	if chaos == nil || (len(chaos.FatalFlaws) == 0 && len(chaos.Constraints) == 0 && len(chaos.RejectedPatterns) == 0 && len(chaos.StressScenarios) == 0) {
		slog.Info("aporia_engine: sparse or missing chaos analysis, generating deterministic chaos")
		chaos = generateDeterministicChaos(store, session)
		session.ChaosAnalysis = chaos // ensure it persists downstream
	}

	// 2. Build the Heuristic Baseline (Golden Path)
	// Even if we have an LLM, this populates standard-derived decisions and deviation challenges
	synthesis = goldenPathSynthesis(store, session, thesis, antithesis, synthesis)

	// Step 1: Constraint Filtering against BuntDB standards
	filterConstraints(session, chaos, synthesis)

	// Step 2: Decision Matrix — evaluate fatal flaws
	evaluateDecisionMatrix(chaos, synthesis)

	// Step 3: Auto-resolve questions from ingested standards
	// This targets Standard Deviation questions that are false positives — the dependency
	// or pattern IS mentioned in the standards but wasn't caught by naive keyword matching.
	if len(synthesis.OutstandingQuestions) > 0 && len(session.Standards) > 0 {
		before := len(synthesis.OutstandingQuestions)
		synthesis.OutstandingQuestions = resolveQuestionsFromStandards(
			synthesis.OutstandingQuestions, session.Standards, synthesis,
		)
		if resolved := before - len(synthesis.OutstandingQuestions); resolved > 0 {
			slog.Info("aporia_engine: standards auto-resolved deviation questions",
				"resolved", resolved,
				"remaining", len(synthesis.OutstandingQuestions),
			)
		}
	}

	// Step 4: LLM Enhancement (auto-enabled when configured)
	// The LLM will take the heuristic baseline and enhance it further
	llmClient, err := integration.NewLLMClient(store)
	if err == nil && llmClient != nil {
		enhanced, err := llmEnhanceSynthesis(ctx, llmClient, session, thesis, antithesis, chaos, synthesis)
		if err == nil {
			synthesis = enhanced
			synthesis.LLMEnhanced = true
			slog.Info("aporia_engine: LLM synthesis applied successfully over heuristic baseline")
		} else {
			slog.Warn("aporia_engine: LLM synthesis failed at runtime, using deterministic fallback",
				"error", err,
			)
		}
	} else if errors.Is(err, integration.ErrLLMDisabled) {
		slog.Info("aporia_engine: Intelligence Engine disabled via config, using heuristic baseline")
	} else {
		slog.Debug("aporia_engine: LLM not configured, using heuristic baseline")
	}

	// Step 5: Targeted LLM resolution of remaining questions
	// This is SEPARATE from the full synthesis enhancement — it specifically asks the LLM
	// to resolve or mark as HUMAN_REQUIRED each outstanding question.
	if len(synthesis.OutstandingQuestions) > 0 && llmClient != nil {
		before := len(synthesis.OutstandingQuestions)
		synthesis.OutstandingQuestions = resolveQuestionsViaLLM(
			ctx, llmClient, synthesis.OutstandingQuestions, session, synthesis,
		)
		if resolved := before - len(synthesis.OutstandingQuestions); resolved > 0 {
			slog.Info("aporia_engine: LLM targeted resolution resolved questions",
				"resolved", resolved,
				"remaining", len(synthesis.OutstandingQuestions),
			)
		}
	}

	// Import Chaos data into synthesis
	synthesis.ChaosVetted = len(chaos.FatalFlaws) == 0 || allFatalFlawsResolved(chaos, synthesis)
	synthesis.RejectedOptions = chaos.RejectedPatterns
	synthesis.ConstraintLocks = chaos.Constraints

	// Phase 2 Enhancement: Persist any new rejected patterns into the Chaos Graveyard for this stack
	if len(synthesis.RejectedOptions) > 0 {
		if err := store.SaveChaosGraveyard(session.TechStack, synthesis.RejectedOptions); err != nil {
			slog.Warn("aporia_engine: failed to persist chaos graveyard", "error", err)
		} else {
			slog.Debug("aporia_engine: updated chaos graveyard", "stack", session.TechStack)
		}
	}

	return synthesis
}

// filterConstraints checks each Chaos constraint against BuntDB standards.
// If a matching standard exists, the constraint is auto-resolved (self-healing).
func filterConstraints(
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

// resolveQuestionsFromStandards checks each outstanding question against the
// ingested standard content stored in the session. Standard Deviation questions
// are the primary target: if the flagged dependency/keyword appears in any
// standard's content, the deviation is a false positive and is auto-resolved
// with a decision citing the standard.
func resolveQuestionsFromStandards(
	questions []db.GranularQuestion,
	standards []string,
	synthesis *db.SynthesisResolution,
) []db.GranularQuestion {
	if len(questions) == 0 || len(standards) == 0 {
		return questions
	}

	// Build a single lowercase corpus of all standard content for efficient matching
	standardsCorpus := strings.ToLower(strings.Join(standards, "\n"))

	var remaining []db.GranularQuestion
	for _, q := range questions {
		topicLower := strings.ToLower(q.Topic)

		// Extract the keyword to search for based on question type
		var searchKeyword string

		if strings.Contains(topicLower, "standard deviation") {
			// Standard Deviation questions follow patterns like:
			// "Standard Deviation: Module → @dep" or "Standard Deviation: Stack Tuning [Cat]"
			// Extract the dependency/keyword after the arrow or brackets
			if idx := strings.Index(q.Topic, "→"); idx != -1 {
				searchKeyword = strings.TrimSpace(q.Topic[idx+len("→"):])
			} else if idx := strings.Index(q.Topic, "["); idx != -1 {
				end := strings.Index(q.Topic[idx:], "]")
				if end != -1 {
					searchKeyword = strings.TrimSpace(q.Topic[idx+1 : idx+end])
				}
			}
		}

		// Only auto-resolve if we found a specific keyword to search for
		if searchKeyword != "" && strings.Contains(standardsCorpus, strings.ToLower(searchKeyword)) {
			synthesis.Decisions = append(synthesis.Decisions, db.ArchitecturalDecision{
				Topic:     q.Topic,
				Decision:  fmt.Sprintf("Auto-approved: '%s' is referenced in ingested standards content", searchKeyword),
				Rationale: "Standard content addresses this technology/pattern. Deviation is a false positive.",
			})
			slog.Info("aporia_engine: deviation question auto-resolved from standards",
				"topic", q.Topic,
				"keyword", searchKeyword,
			)
			continue
		}

		remaining = append(remaining, q)
	}

	return remaining
}

// llmQuestionResolution is the structured response format for targeted LLM question resolution.
type llmQuestionResolution struct {
	Resolutions []struct {
		Topic        string `json:"topic"`
		Decision     string `json:"decision"`
		Rationale    string `json:"rationale"`
		HumanRequired bool   `json:"human_required"`
		HumanReason  string `json:"human_reason,omitempty"`
	} `json:"resolutions"`
}

// resolveQuestionsViaLLM performs targeted question resolution using the LLM.
// For each outstanding question, the LLM either provides a concrete architectural
// decision or marks it as HUMAN_REQUIRED. Only HUMAN_REQUIRED questions remain
// outstanding after this stage.
func resolveQuestionsViaLLM(
	ctx context.Context,
	client *llm.Client,
	questions []db.GranularQuestion,
	session *db.SessionState,
	synthesis *db.SynthesisResolution,
) []db.GranularQuestion {
	if len(questions) == 0 {
		return questions
	}

	// Build a focused prompt for targeted question resolution
	var prompt strings.Builder
	prompt.WriteString("You are a Senior Architectural Decision Evaluator with deep expertise in ")
	prompt.WriteString(session.TechStack)
	prompt.WriteString(" systems. The following outstanding questions ")
	prompt.WriteString("emerged during architectural vetting of a ")
	prompt.WriteString(session.TechStack)
	prompt.WriteString(" project.\n\n")

	// Add project context so LLM understands the domain
	if session.OriginalIdea != "" {
		prompt.WriteString("PROJECT CONTEXT:\n")
		prompt.WriteString(truncateString(session.OriginalIdea, 500))
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("DIRECTIVE: You MUST resolve as many questions as possible yourself. ")
	prompt.WriteString("You are the expert — use your knowledge of industry best practices, ")
	prompt.WriteString("documented patterns, and technical tradeoffs to make concrete decisions.\n\n")
	prompt.WriteString("For EACH question below, you MUST either:\n")
	prompt.WriteString("1. RESOLVE IT (set human_required=false, provide decision+rationale) — ")
	prompt.WriteString("this is the STRONGLY PREFERRED option for ALL technical questions including: ")
	prompt.WriteString("performance tradeoffs, memory management strategies, caching strategies, ")
	prompt.WriteString("error handling approaches, API design choices, parser selection, ")
	prompt.WriteString("concurrency models, timeout values, and technology compatibility.\n")
	prompt.WriteString("2. Mark as HUMAN_REQUIRED (set human_required=true, provide human_reason) — ")
	prompt.WriteString("ONLY for questions that genuinely require HUMAN BUSINESS JUDGMENT such as: ")
	prompt.WriteString("budget allocation, team staffing decisions, regulatory compliance choices, ")
	prompt.WriteString("go-to-market timing, vendor contract negotiations, or organizational policy.\n\n")
	prompt.WriteString("CRITICAL: If a question has a clear technical best practice or a reasonable ")
	prompt.WriteString("default, YOU MUST resolve it yourself. Do NOT mark technical questions as HUMAN_REQUIRED.\n\n")

	// Include existing decisions so LLM knows what's already been resolved
	if len(synthesis.Decisions) > 0 {
		prompt.WriteString("EXISTING DECISIONS (already resolved — do not contradict these):\n")
		for i, d := range synthesis.Decisions {
			if i >= 10 {
				prompt.WriteString(fmt.Sprintf("... and %d more decisions\n", len(synthesis.Decisions)-10))
				break
			}
			prompt.WriteString(fmt.Sprintf("  - %s: %s\n", d.Topic, d.Decision))
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString("OUTSTANDING QUESTIONS TO RESOLVE:\n")
	for i, q := range questions {
		prompt.WriteString(fmt.Sprintf("\n%d. Topic: %s\n   Question: %s\n   Context: %s\n   Impact: %s\n",
			i+1, q.Topic, q.Question, q.Context, q.Impact))
	}

	// Use a shorter timeout for targeted resolution
	llmCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var result llmQuestionResolution
	if err := client.JSONResponse(llmCtx, prompt.String(), &result); err != nil {
		slog.Warn("aporia_engine: targeted LLM question resolution failed, all questions remain",
			"error", err,
			"question_count", len(questions),
		)
		return questions
	}

	// Build a topic→resolution lookup for matching
	resolutionMap := make(map[string]*struct {
		Decision     string
		Rationale    string
		HumanRequired bool
		HumanReason  string
	})
	for i := range result.Resolutions {
		r := &result.Resolutions[i]
		resolutionMap[strings.ToLower(r.Topic)] = &struct {
			Decision     string
			Rationale    string
			HumanRequired bool
			HumanReason  string
		}{r.Decision, r.Rationale, r.HumanRequired, r.HumanReason}
	}

	var remaining []db.GranularQuestion
	for _, q := range questions {
		r, found := resolutionMap[strings.ToLower(q.Topic)]
		if !found {
			// LLM didn't address this question — keep it outstanding
			remaining = append(remaining, q)
			continue
		}

		if r.HumanRequired {
			// LLM explicitly says this needs human input — keep it but annotate
			if r.HumanReason != "" {
				q.Context = q.Context + " [LLM Note: " + r.HumanReason + "]"
			}
			remaining = append(remaining, q)
			slog.Debug("aporia_engine: LLM marked question as HUMAN_REQUIRED",
				"topic", q.Topic,
				"reason", r.HumanReason,
			)
			continue
		}

		// LLM resolved this question — create a decision
		synthesis.Decisions = append(synthesis.Decisions, db.ArchitecturalDecision{
			Topic:     q.Topic,
			Decision:  r.Decision,
			Rationale: r.Rationale + " [resolved by LLM]",
		})
		slog.Info("aporia_engine: LLM resolved question",
			"topic", q.Topic,
		)
	}

	return remaining
}

// evaluateDecisionMatrix applies the tri-factor scoring logic.
// Fatal flaws that cannot be resolved trigger True Aporia (escalation to user)
// with category-specific, quantitative questions instead of generic ones.
func evaluateDecisionMatrix(
	chaos *db.ChaosAnalysis,
	synthesis *db.SynthesisResolution,
) {
	if len(chaos.FatalFlaws) == 0 {
		return
	}

	for _, flaw := range chaos.FatalFlaws {
		// Skip generating the question if a decision already addresses this flaw
		if decisionCovers(synthesis.Decisions, flaw.Category) {
			continue
		}

		// Generate category-specific smart questions with quantitative prompts
		question, impact := chaosFlawToSmartQuestion(flaw)

		synthesis.OutstandingQuestions = append(synthesis.OutstandingQuestions, db.GranularQuestion{
			Topic:    fmt.Sprintf("Chaos Fatal Flaw [%s]", flaw.Category),
			Question: question,
			Context:  fmt.Sprintf("The Chaos Architect identified a %s-severity flaw: %s", flaw.Severity, flaw.MitigationStrategy),
			Impact:   impact,
		})
	}
}

// allFatalFlawsResolved checks if every fatal flaw has been addressed
// by a constraint filter or an existing decision using bidirectional
// partial string matching.
func allFatalFlawsResolved(chaos *db.ChaosAnalysis, synthesis *db.SynthesisResolution) bool {
	if len(chaos.FatalFlaws) == 0 {
		return true
	}

	for _, flaw := range chaos.FatalFlaws {
		if !decisionCovers(synthesis.Decisions, flaw.Category) {
			return false
		}
	}
	return true
}

// decisionCovers checks if any existing decision addresses the given category
// using bidirectional case-insensitive partial string matching across both
// the Topic and Decision fields. This is the canonical decision-lookup used
// by evaluateDecisionMatrix, allFatalFlawsResolved, and goldenPathSynthesis.
func decisionCovers(decisions []db.ArchitecturalDecision, category string) bool {
	categoryLower := strings.ToLower(category)
	if len(categoryLower) < 3 {
		return false // Too short to match meaningfully
	}
	for _, d := range decisions {
		topicLower := strings.ToLower(d.Topic)
		decisionLower := strings.ToLower(d.Decision)
		if strings.Contains(topicLower, categoryLower) ||
			strings.Contains(categoryLower, topicLower) ||
			strings.Contains(decisionLower, categoryLower) {
			return true
		}
	}
	return false
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
				allDecisions := append(baseSynthesis.Decisions, enhanced.Decisions...)
				allQuestions := mergeQuestions(baseSynthesis.OutstandingQuestions, enhanced.OutstandingQuestions)

				// Post-merge filter: remove any outstanding questions (including new
				// LLM-generated ones) that are already addressed by an existing
				// decision. Without this, the LLM can inject new questions on every
				// invocation that the agent's decisions already cover, creating an
				// infinite Socratic loop.
				filtered := make([]db.GranularQuestion, 0, len(allQuestions))
				for _, q := range allQuestions {
					if !decisionCovers(allDecisions, q.Topic) {
						filtered = append(filtered, q)
					}
				}

				if len(filtered) < len(allQuestions) {
					slog.Info("aporia_engine: post-merge filter removed LLM questions covered by decisions",
						"before", len(allQuestions),
						"after", len(filtered),
						"removed", len(allQuestions)-len(filtered),
					)
				}

				merged := &db.SynthesisResolution{
					Narrative:              enhanced.Narrative,
					Decisions:              allDecisions,
					OutstandingQuestions:    filtered,
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

// ----- WP-2A: Golden Path Deterministic Synthesis -----

// goldenPathSynthesis generates a complete synthesis from BuntDB standards and the
// design proposal when both LLM and Chaos Analysis are absent. This replaces the
// previous "pass-through" behavior with an Expert System that produces meaningful
// architectural decisions from the ingested standards alone.
//
// The Golden Path selects the MOST RESTRICTIVE standard interpretation for safety,
// then generates decisions affirming each module, security mandate, and stack tuning
// recommendation against the authoritative standards.
func goldenPathSynthesis(
	store *db.Store,
	session *db.SessionState,
	thesis *db.DesignProposal,
	antithesis *db.SkepticAnalysis,
	agentSynthesis *db.SynthesisResolution,
) *db.SynthesisResolution {
	// Suppress unused parameter warning — store is reserved for future BuntDB standard lookups
	_ = store

	synthesis := agentSynthesis
	if synthesis == nil {
		synthesis = &db.SynthesisResolution{}
	}

	// hasDecision delegates to the canonical decisionCovers helper
	hasDecision := func(category string) bool {
		return decisionCovers(synthesis.Decisions, category)
	}

	var narrativeParts []string
	narrativeParts = append(narrativeParts, "Golden Path Synthesis — Deterministic resolution using ingested standards as the authoritative source.")

	// 1. Module-level decisions: affirm each proposed module against standards
	if thesis != nil && len(thesis.ProposedModules) > 0 {
		for _, mod := range thesis.ProposedModules {
			synthesis.Decisions = append(synthesis.Decisions, db.ArchitecturalDecision{
				Topic:     fmt.Sprintf("Module: %s", mod.Name),
				Decision:  fmt.Sprintf("Approved for implementation with %d responsibilities", len(mod.Responsibilities)),
				Rationale: fmt.Sprintf("[Golden Path] Module '%s' (%s) follows separation of concerns with %d declared dependencies", mod.Name, mod.Purpose, len(mod.Dependencies)),
			})
		}
		narrativeParts = append(narrativeParts, fmt.Sprintf("Affirmed %d modules from the design proposal.", len(thesis.ProposedModules)))
	}

	// 2. Security decisions: affirm each security mandate
	if thesis != nil && len(thesis.SecurityMandates) > 0 {
		for _, mandate := range thesis.SecurityMandates {
			synthesis.Decisions = append(synthesis.Decisions, db.ArchitecturalDecision{
				Topic:     fmt.Sprintf("Security: %s", mandate.Category),
				Decision:  mandate.Description,
				Rationale: fmt.Sprintf("[Golden Path] Mandated at %s severity. Mitigation: %s", mandate.Severity, mandate.MitigationStrategy),
			})
		}
		narrativeParts = append(narrativeParts, fmt.Sprintf("Enforced %d security mandates.", len(thesis.SecurityMandates)))
	}

	// 3. Stack tuning decisions: affirm each recommendation
	if thesis != nil && len(thesis.StackTuning) > 0 {
		for _, opt := range thesis.StackTuning {
			synthesis.Decisions = append(synthesis.Decisions, db.ArchitecturalDecision{
				Topic:     fmt.Sprintf("Stack Tuning: %s", opt.Category),
				Decision:  opt.Recommendation,
				Rationale: fmt.Sprintf("[Golden Path] %s priority. %s", opt.Priority, opt.Rationale),
			})
		}
		narrativeParts = append(narrativeParts, fmt.Sprintf("Applied %d stack tuning recommendations.", len(thesis.StackTuning)))
	}

	// 4. Standard-deviation challenge questions (Rec #1)
	if thesis != nil && len(session.Standards) > 0 {
		deviationQuestions := generateStandardDeviationQuestions(session, thesis)
		for _, q := range deviationQuestions {
			if agentSynthesis == nil || len(agentSynthesis.Decisions) == 0 || !hasDecision(q.Topic) {
				synthesis.OutstandingQuestions = append(synthesis.OutstandingQuestions, q)
			}
		}
		if len(deviationQuestions) > 0 {
			narrativeParts = append(narrativeParts, fmt.Sprintf("Identified %d potential standard deviations.", len(deviationQuestions)))
		}
	}

	// 5. Skeptic-informed questions: surface antithesis concerns that are NOT already
	// addressed by an existing decision. Uses per-question filtering instead of
	// all-or-nothing gating to avoid silently dropping unresolved concerns.
	if antithesis != nil {
		for _, vuln := range antithesis.Vulnerabilities {
			if !hasDecision(vuln.Category) {
				synthesis.OutstandingQuestions = append(synthesis.OutstandingQuestions, db.GranularQuestion{
					Topic:    fmt.Sprintf("Skeptic Vulnerability [%s]", vuln.Category),
					Question: fmt.Sprintf("The Antithesis Skeptic identified: %s. How will this be mitigated?", vuln.Description),
					Context:  fmt.Sprintf("Proposed mitigation: %s", vuln.MitigationStrategy),
					Impact:   fmt.Sprintf("Severity: %s — unmitigated vulnerability may compromise production safety", vuln.Severity),
				})
			}
		}
		for _, concern := range antithesis.DesignConcerns {
			if !hasDecision(concern.Area) {
				synthesis.OutstandingQuestions = append(synthesis.OutstandingQuestions, db.GranularQuestion{
					Topic:    fmt.Sprintf("Design Concern [%s]", concern.Area),
					Question: fmt.Sprintf("The Antithesis Skeptic flagged: %s. Should the architecture be adjusted?", concern.Concern),
					Context:  fmt.Sprintf("Suggestion: %s", concern.Suggestion),
					Impact:   fmt.Sprintf("Severity: %s — design concern may affect maintainability and scalability", concern.Severity),
				})
			}
		}
	}

	synthesis.Narrative = strings.Join(narrativeParts, " ")

	slog.Info("aporia_engine: Golden Path synthesis completed",
		"decisions", len(synthesis.Decisions),
		"outstanding_questions", len(synthesis.OutstandingQuestions),
	)

	return synthesis
}

// ----- WP-2B: Standard-Deviation Challenge Questions -----

// generateStandardDeviationQuestions detects conflicts between the ingested standards
// text and the design proposal. When a module dependency or stack tuning item mentions
// a technology that DOESN'T appear in the standards, the engine generates a targeted
// question asking the user to justify the deviation.
func generateStandardDeviationQuestions(session *db.SessionState, thesis *db.DesignProposal) []db.GranularQuestion {
	if len(session.Standards) == 0 || thesis == nil {
		return nil
	}

	standardsLower := strings.ToLower(strings.Join(session.Standards, "\n"))
	var questions []db.GranularQuestion

	// Check each module's dependencies for standard deviations
	for _, mod := range thesis.ProposedModules {
		for _, dep := range mod.Dependencies {
			depLower := strings.ToLower(dep)
			if len(depLower) > 3 && !strings.Contains(standardsLower, depLower) {
				questions = append(questions, db.GranularQuestion{
					Topic:    fmt.Sprintf("Standard Deviation: %s → %s", mod.Name, dep),
					Question: fmt.Sprintf("Module '%s' declares dependency on '%s', which does not appear in the ingested standards. Should we deviate from the standard, and what is the technical justification?", mod.Name, dep),
					Context:  fmt.Sprintf("Ingested standards cover the %s stack but do not reference '%s'", session.TechStack, dep),
					Impact:   "Non-standard dependencies may introduce maintenance burden, security risk, or compatibility issues",
				})
			}
		}
	}

	// Check stack tuning for non-standard recommendations
	for _, opt := range thesis.StackTuning {
		keywords := extractSearchTerms(opt.Recommendation)
		for _, kw := range keywords {
			if len(kw) > 4 && !strings.Contains(standardsLower, kw) {
				questions = append(questions, db.GranularQuestion{
					Topic:    fmt.Sprintf("Standard Deviation: Stack Tuning [%s]", opt.Category),
					Question: fmt.Sprintf("Stack tuning recommends '%s' which references '%s' not found in ingested standards. Is this an intentional deviation?", opt.Recommendation, kw),
					Context:  fmt.Sprintf("Category: %s, Priority: %s", opt.Category, opt.Priority),
					Impact:   "Non-standard tuning may require additional validation and documentation",
				})
				break // One question per recommendation is sufficient
			}
		}
	}

	// Cap at 5 deviation questions to avoid question fatigue
	if len(questions) > 5 {
		questions = questions[:5]
	}

	return questions
}

// ----- WP-2C: Chaos-Informed Smart Questions -----

// chaosFlawToSmartQuestion generates a category-specific, quantitative question
// from a Chaos Architect fatal flaw. Instead of generic "may need redesign" questions,
// this provides actionable, measurable prompts that bridge the gap between the
// abstract risk and a concrete architectural decision.
func chaosFlawToSmartQuestion(flaw db.SecurityItem) (question, impact string) {
	category := strings.ToLower(flaw.Category)

	switch {
	case strings.Contains(category, "resource") || strings.Contains(category, "exhaustion") || strings.Contains(category, "memory"):
		question = fmt.Sprintf("The Chaos Architect identified resource exhaustion risk: %s. "+
			"What is the memory ceiling for this service? (256MB / 512MB / 1GB) "+
			"Should we implement an in-memory buffer with backpressure, or accept the I/O latency?", flaw.Description)
		impact = "Uncontrolled resource usage will trigger OOM kills in containerized environments and degrade collocated services"

	case strings.Contains(category, "concurren") || strings.Contains(category, "parallel") || strings.Contains(category, "race"):
		question = fmt.Sprintf("The Chaos Architect identified concurrency risk: %s. "+
			"What is the expected concurrent connection count? (10 / 100 / 1000) "+
			"Should we implement connection pooling with bounded workers, or use unbounded goroutines/promises?", flaw.Description)
		impact = "Unbounded concurrency will cause cascading failures under load and non-deterministic behavior"

	case strings.Contains(category, "file") || strings.Contains(category, "lock") || strings.Contains(category, "disk") || strings.Contains(category, "io"):
		question = fmt.Sprintf("The Chaos Architect identified I/O contention risk: %s. "+
			"Should we implement an in-memory buffer with periodic flush, or use direct file I/O with advisory locking? "+
			"What is the acceptable write latency? (1ms / 10ms / 100ms)", flaw.Description)
		impact = "File locking contention will cause deadlocks and data corruption under concurrent write scenarios"

	case strings.Contains(category, "network") || strings.Contains(category, "timeout") || strings.Contains(category, "latency"):
		question = fmt.Sprintf("The Chaos Architect identified network reliability risk: %s. "+
			"What is the acceptable request timeout? (5s / 15s / 30s) "+
			"Should we implement circuit breakers with exponential backoff, or fail-fast with retry at the caller?", flaw.Description)
		impact = "Network timeouts without circuit breakers will cause thread/connection pool exhaustion and cascade failures"

	case strings.Contains(category, "security") || strings.Contains(category, "auth") || strings.Contains(category, "crypto"):
		question = fmt.Sprintf("The Chaos Architect identified a security flaw: %s. "+
			"Should we implement defense-in-depth (multiple layers of validation), or a single trust boundary with strict enforcement? "+
			"What is the threat model: internal-only, authenticated-external, or public-internet?", flaw.Description)
		impact = "Unresolved security flaws create attack surface that cannot be retroactively hardened without architectural changes"

	case strings.Contains(category, "data") || strings.Contains(category, "state") || strings.Contains(category, "persist"):
		question = fmt.Sprintf("The Chaos Architect identified data integrity risk: %s. "+
			"What is the consistency requirement? (eventual / strong / linearizable) "+
			"What is the acceptable data loss window? (0 / 1s / 5min)", flaw.Description)
		impact = "Data integrity flaws compound over time and are catastrophically expensive to remediate post-deployment"

	default:
		question = fmt.Sprintf("The Chaos Architect identified a %s-severity flaw: %s. "+
			"What is your risk tolerance for this category? Should we apply the most conservative mitigation (%s), "+
			"or accept the risk with monitoring?", flaw.Severity, flaw.Description, flaw.MitigationStrategy)
		impact = fmt.Sprintf("Unresolved %s flaw may require architectural changes later — addressing it now is significantly cheaper", flaw.Category)
	}

	return question, impact
}

// ----- WP-2D: Deterministic Chaos Generation -----

// generateDeterministicChaos constructs a baseline ChaosAnalysis object based on the
// target environment and tech stack. This ensures the pipeline ALWAYS has
// architectural guardrails (fatal flaws, constraints, rejected patterns) even if
// the agent/LLM fails to provide them or is completely disabled.
func generateDeterministicChaos(store *db.Store, session *db.SessionState) *db.ChaosAnalysis {
	chaos := &db.ChaosAnalysis{
		ChaosScore: 7, // Default baseline assuming some inherent complexity
		Narrative:  "Deterministic Chaos Generation: The agent provided sparse or nil chaos data. Auto-generating baseline architectural guardrails based on the target environment and stack.",
	}

	// 1. Environment-Specific Fatal Flaws
	switch session.TargetEnvironment {
	case "containerized", "kubernetes":
		chaos.FatalFlaws = append(chaos.FatalFlaws, db.SecurityItem{
			Category:           "Resource Exhaustion",
			Description:        "Unbounded memory allocation leading to OOMKills in containerized environments",
			Severity:           "critical",
			MitigationStrategy: "Implement strict memory ceilings and backpressure on streams",
		})
	case "local-ide":
		chaos.FatalFlaws = append(chaos.FatalFlaws, db.SecurityItem{
			Category:           "Concurrency",
			Description:        "Synchronous blocking operations causing JSON-RPC timeouts",
			Severity:           "high",
			MitigationStrategy: "Offload CPU-bound tasks to Worker threads or child processes",
		})
	}

	// 2. Baseline Constraints
	chaos.Constraints = append(chaos.Constraints, db.ChaosConstraint{
		Domain:     "Runtime",
		Constraint: "No native add-ons unless pre-compiled for all target architectures",
		Platform:   session.TechStack,
		Impact:     "Prevents deployment failures due to missing build tools",
	})

	// 3. The Chaos Graveyard (Rejected Patterns)
	// These patterns are universally unsafe and must not be used.
	chaos.RejectedPatterns = append(chaos.RejectedPatterns, db.ChaosRejection{
		Pattern:  "Synchronous file system operations in the main event loop",
		Reason:   "Blocks all concurrent request handling, causing cascading timeouts",
		Severity: "critical",
		Source:   "chaos_architect",
	})
	chaos.RejectedPatterns = append(chaos.RejectedPatterns, db.ChaosRejection{
		Pattern:  "Unbounded in-memory arrays for data processing",
		Reason:   "Causes predictable OOM crashes on large datasets",
		Severity: "high",
		Source:   "chaos_architect",
	})

	// 4. BuntDB Historical Graveyard Injection
	// Pull any historically rejected patterns for this tech stack
	if historical, err := store.GetChaosGraveyard(session.TechStack); err == nil && len(historical) > 0 {
		chaos.RejectedPatterns = append(chaos.RejectedPatterns, historical...)
		slog.Info("aporia_engine: injected historical chaos graveyard patterns", "count", len(historical), "stack", session.TechStack)
	}

	slog.Info("aporia_engine: generated deterministic chaos",
		"fatal_flaws", len(chaos.FatalFlaws),
		"constraints", len(chaos.Constraints),
		"rejected_patterns", len(chaos.RejectedPatterns),
	)

	return chaos
}
