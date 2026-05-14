// Package socratic provides functionality for the socratic subsystem.
package socratic

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Stage defines the Stage structure.
type Stage string

const (
	StageIdle               Stage = "IDLE"
	StageThesis             Stage = "THESIS"
	StageAntithesisInitial  Stage = "ANTITHESIS_INITIAL"
	StageThesisDefense      Stage = "THESIS_DEFENSE"
	StageAntithesisEvaluate Stage = "ANTITHESIS_EVALUATE"
	StageChaos              Stage = "CHAOS"
	StageAporia             Stage = "APORIA"

	// minDialecticRounds is the minimum number of thesis/antithesis rounds before
	// the convergence gate allows is_satisfied=true to pass. This prevents the
	// LLM from taking the path of least resistance on the first evaluation.
	minDialecticRounds = 3

	// maxDialecticRounds is the absolute cap preventing infinite dialectic loops.
	// At round 6, the pipeline forces convergence regardless of satisfaction.
	maxDialecticRounds = 6

	// maxChaosRounds caps the number of Chaos stress-test cycles. After this
	// many Black Swan challenges, the pipeline proceeds to APORIA.
	maxChaosRounds = 2
)

// LemmaEntry stores a lemma with server-side metadata for programmatic consumption.
// The agent provides only the Text; the server tags Stage and Round at append time.
type LemmaEntry struct {
	Stage Stage  // which pipeline stage produced this lemma
	Round int    // which dialectic/chaos round
	Text  string // the 2-sentence lemma from the agent
}

// PipelineState holds the state of a single dialectic session.
type PipelineState struct {
	OriginalPrompt  string
	LemmaTrail      []LemmaEntry
	CurrentStage    Stage
	DeadlockCount   int
	DialecticRound  int
	ChaosRound      int  // tracks Chaos stress-test iterations (max 2)
	InChaosRebuttal bool // distinguishes pre-Chaos and post-Chaos dialectic phases
	ContextBytes    int
	TokensEst       int
}

// cachedMetrics stores the last successfully read metrics for non-blocking telemetry.
type cachedMetrics struct {
	stage           string
	trifectaReviews int
	contextBytes    int
	tokensEst       int
}

// Machine manages the single global pipeline.
type Machine struct {
	mu          sync.Mutex
	pipeline    *PipelineState
	lastMetrics cachedMetrics
}

// NewMachine creates a new Socratic Machine.
func NewMachine() *Machine {
	return &Machine{
		pipeline: &PipelineState{CurrentStage: StageIdle},
	}
}

// Request defines the stripped JSON payload expected from the tool.
type Request struct {
	Stage              string `json:"stage" validate:"required,oneof=INITIALIZE THESIS ANTITHESIS_INITIAL THESIS_DEFENSE ANTITHESIS_EVALUATE CHAOS APORIA RESET"`
	Problem            string `json:"problem,omitempty"`
	Lemma              string `json:"lemma,omitempty"`
	AporiaSynthesis    string `json:"aporia_synthesis,omitempty"`
	SynthesisCritique  string `json:"synthesis_critique,omitempty"`
	ParadoxDetected    bool   `json:"paradox_detected,omitempty"`
	ResolutionStrategy string `json:"resolution_strategy,omitempty"`
	IsSatisfied        bool   `json:"is_satisfied,omitempty"`
}

// Process runs the input request through the state machine.
// It explicitly checks context.Context to prevent holding the mutex if the client cancels.
func (m *Machine) Process(ctx context.Context, req Request) (string, error) {
	// Attempt to acquire lock, respecting context cancellation
	lockAcquired := make(chan struct{})
	go func() {
		m.mu.Lock()
		close(lockAcquired)
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-lockAcquired:
		defer m.mu.Unlock()
	}

	// 1. Measure incoming text drag BEFORE validation rules drop the payload
	incomingTextDrag := len(req.Problem) + len(req.Lemma) + len(req.AporiaSynthesis) + len(req.SynthesisCritique) + len(req.ResolutionStrategy)
	m.pipeline.ContextBytes += incomingTextDrag
	m.pipeline.TokensEst = m.pipeline.ContextBytes / 4

	// Helper to track outgoing drag
	trackOutgoing := func(out string) string {
		m.pipeline.ContextBytes += len(out)
		m.pipeline.TokensEst = m.pipeline.ContextBytes / 4
		return out
	}

	// Always allow hard reset (preserves current context drag to observe aborted sessions)
	if req.Stage == "RESET" {
		m.pipeline = &PipelineState{
			CurrentStage: StageIdle,
			ContextBytes: m.pipeline.ContextBytes,
			TokensEst:    m.pipeline.TokensEst,
		}
		return trackOutgoing("Pipeline reset. Please submit INITIALIZE with your raw problem to start a new Socratic session."), nil
	}

	var out string
	var err error

	switch m.pipeline.CurrentStage {
	case StageIdle:
		if req.Stage != "INITIALIZE" {
			out, err = m.formatError("INITIALIZE"), errors.New("invalid stage")
		} else {
			out, err = m.initialize(req.Problem)
		}
	case StageAporia:
		if req.Stage == "INITIALIZE" {
			out, err = m.initialize(req.Problem)
		} else if req.Stage != "APORIA" {
			out, err = m.formatError("APORIA"), errors.New("invalid stage")
		} else {
			out, err = m.handleAporia(req)
		}
	case StageThesis:
		if req.Stage != "THESIS" {
			out, err = m.formatError("THESIS"), errors.New("invalid stage")
		} else {
			out, err = m.handleThesis(req.Lemma)
		}
	case StageAntithesisInitial:
		if req.Stage != "ANTITHESIS_INITIAL" {
			out, err = m.formatError("ANTITHESIS_INITIAL"), errors.New("invalid stage")
		} else {
			out, err = m.handleAntithesisInitial(req.Lemma)
		}
	case StageThesisDefense:
		if req.Stage != "THESIS_DEFENSE" {
			out, err = m.formatError("THESIS_DEFENSE"), errors.New("invalid stage")
		} else {
			out, err = m.handleThesisDefense(req.Lemma)
		}
	case StageAntithesisEvaluate:
		if req.Stage != "ANTITHESIS_EVALUATE" {
			out, err = m.formatError("ANTITHESIS_EVALUATE"), errors.New("invalid stage")
		} else {
			out, err = m.handleAntithesisEvaluate(req.Lemma, req.IsSatisfied)
		}
	case StageChaos:
		if req.Stage != "CHAOS" {
			out, err = m.formatError("CHAOS"), errors.New("invalid stage")
		} else {
			out, err = m.handleChaos(req.Lemma)
		}
	default:
		out, err = "", fmt.Errorf("unknown pipeline stage: %s", m.pipeline.CurrentStage)
	}

	return trackOutgoing(out), err
}

func (m *Machine) initialize(problem string) (string, error) {
	// A new INITIALIZE implies a new session; explicitly reset context tracking
	m.pipeline = &PipelineState{
		OriginalPrompt: problem,
		CurrentStage:   StageThesis,
		LemmaTrail:     []LemmaEntry{},
		DialecticRound: 0,
		ChaosRound:     0,
		ContextBytes:   0,
		TokensEst:      0,
	}
	return `{"stage_accepted": "INITIALIZE", "next_stage": "THESIS", "directive": "You are the Thesis Architect. Provide a clear, robust initial solution or hypothesis to the user's problem natively in your thought block. You MUST address ALL of these dimensions: (1) the core mechanism or approach, (2) at least TWO alternative approaches considered and why they were rejected, (3) the operational and deployment implications, (4) the security and safety posture, (5) at least ONE primary risk with a concrete mitigation strategy. Once reached, distill into exactly TWO SENTENCES: your position, then the specific technical evidence or mechanism supporting it. Call the tool with stage=THESIS."}`, nil
}

func (m *Machine) handleThesis(lemma string) (string, error) {
	if strings.TrimSpace(lemma) == "" {
		return m.formatError("THESIS"), errors.New("missing lemma")
	}

	m.pipeline.LemmaTrail = append(m.pipeline.LemmaTrail, LemmaEntry{
		Stage: StageThesis,
		Round: 1,
		Text:  strings.TrimSpace(lemma),
	})
	m.pipeline.CurrentStage = StageAntithesisInitial
	m.pipeline.DialecticRound = 1

	return `{"stage_accepted": "THESIS", "next_stage": "ANTITHESIS_INITIAL", "directive": "You are the Antithesis Skeptic. Critique your previous Thesis natively in your thought block. You MUST generate challenges across ALL of these categories: (1) Correctness gaps — logical flaws or unsupported assumptions, (2) Security/safety risks — attack vectors or data integrity concerns, (3) Scalability/performance — bottlenecks under load or growth, (4) Edge cases — boundary conditions, empty inputs, concurrent access, (5) The Unasked Question — what critical aspect has the Thesis NOT addressed? (6) Dimensional Omissions — identify at least ONE entire domain or concern the Thesis failed to examine, and explain why its absence matters. Distill into exactly TWO SENTENCES: your strongest challenge, then a specific technical example or failure scenario demonstrating it. Call the tool with stage=ANTITHESIS_INITIAL."}`, nil
}

func (m *Machine) handleAntithesisInitial(lemma string) (string, error) {
	if strings.TrimSpace(lemma) == "" {
		return m.formatError("ANTITHESIS_INITIAL"), errors.New("missing lemma")
	}

	m.pipeline.LemmaTrail = append(m.pipeline.LemmaTrail, LemmaEntry{
		Stage: StageAntithesisInitial,
		Round: m.pipeline.DialecticRound,
		Text:  strings.TrimSpace(lemma),
	})
	m.pipeline.CurrentStage = StageThesisDefense

	return `{"stage_accepted": "ANTITHESIS_INITIAL", "next_stage": "THESIS_DEFENSE", "directive": "You are the Thesis Architect. You MUST defend against EVERY challenge category raised by the Antithesis Skeptic natively in your thought block. Do not concede without providing evidence or structural reasoning. Distill into exactly TWO SENTENCES: your defense, then the specific technical artifact, metric, or mechanism that validates it. Call the tool with stage=THESIS_DEFENSE."}`, nil
}

func (m *Machine) handleThesisDefense(lemma string) (string, error) {
	if strings.TrimSpace(lemma) == "" {
		return m.formatError("THESIS_DEFENSE"), errors.New("missing lemma")
	}

	m.pipeline.LemmaTrail = append(m.pipeline.LemmaTrail, LemmaEntry{
		Stage: StageThesisDefense,
		Round: m.pipeline.DialecticRound,
		Text:  strings.TrimSpace(lemma),
	})
	m.pipeline.CurrentStage = StageAntithesisEvaluate

	// Chaos rebuttal phase uses a different directive
	if m.pipeline.InChaosRebuttal {
		return `{"stage_accepted": "THESIS_DEFENSE", "next_stage": "ANTITHESIS_EVALUATE", "directive": "You are the Antithesis Skeptic. The Thesis Architect has defended against the Chaos Black Swan. Evaluate whether the defense adequately addresses the destabilization natively in your thought block. If the defense holds AND no critical gaps remain, call with stage=ANTITHESIS_EVALUATE, is_satisfied=true. If the defense is insufficient or reveals new vulnerabilities, call with is_satisfied=false. Distill into exactly TWO SENTENCES: your evaluation, then the specific evidence that confirms or refutes the defense."}`, nil
	}

	// Round-indexed evaluation escalation (pre-Chaos dialectic)
	switch {
	case m.pipeline.DialecticRound <= 1:
		// Round 1: Foundation — standard evaluation
		return `{"stage_accepted": "THESIS_DEFENSE", "next_stage": "ANTITHESIS_EVALUATE", "directive": "You are the Antithesis Skeptic. Evaluate the defense natively in your thought block. If satisfied that ALL challenge categories have been adequately addressed, call the tool with stage=ANTITHESIS_EVALUATE, is_satisfied=true. If unsatisfied, generate an 'Increasing Difficulty' prompt targeting Semantic Gaps and the 'Unasked Question', and call with is_satisfied=false. Distill into exactly TWO SENTENCES: your evaluation, then the specific evidence that confirms or refutes the defense."}`, nil
	case m.pipeline.DialecticRound == 2:
		// Round 2: Evidence accountability — demand concrete validation
		return `{"stage_accepted": "THESIS_DEFENSE", "next_stage": "ANTITHESIS_EVALUATE", "directive": "You are the Antithesis Skeptic. Evaluate the defense with HEIGHTENED SCRUTINY natively in your thought block. You MUST evaluate whether the defense provided CONCRETE EVIDENCE (specific technical artifacts, metrics, or mechanisms) for each rebuttal, or merely ASSERTED resolution without validation. Identify any category where the defense relied on reasoning alone without a specific technical example. If satisfied that all rebuttals are evidence-backed, call with is_satisfied=true. If any rebuttal lacks concrete evidence, call with is_satisfied=false. Distill into exactly TWO SENTENCES: your evaluation, then the specific evidence that confirms or refutes the defense."}`, nil
	default:
		// Round 3+: Final challenge — maximum rigor before Chaos
		return `{"stage_accepted": "THESIS_DEFENSE", "next_stage": "ANTITHESIS_EVALUATE", "directive": "You are the Antithesis Skeptic. This is a late-stage evaluation — apply MAXIMUM RIGOR natively in your thought block. Identify the SINGLE MOST DANGEROUS assumption in the consensus that has NOT been stress-tested. Evaluate whether the Thesis has genuinely Steelmanned the opposing view or merely paid lip service to it. If you genuinely cannot find an untested dangerous assumption after rigorous analysis, convergence is justified — call with is_satisfied=true. Otherwise call with is_satisfied=false. Distill into exactly TWO SENTENCES: your evaluation, then the specific evidence that confirms or refutes the defense."}`, nil
	}
}

func (m *Machine) handleAntithesisEvaluate(lemma string, isSatisfied bool) (string, error) {
	if strings.TrimSpace(lemma) == "" {
		return m.formatError("ANTITHESIS_EVALUATE"), errors.New("missing lemma")
	}

	m.pipeline.LemmaTrail = append(m.pipeline.LemmaTrail, LemmaEntry{
		Stage: StageAntithesisEvaluate,
		Round: m.pipeline.DialecticRound,
		Text:  strings.TrimSpace(lemma),
	})

	// --- Chaos Rebuttal Phase ---
	if m.pipeline.InChaosRebuttal {
		// Chaos rebuttal satisfied OR max chaos rounds reached → proceed to APORIA
		if isSatisfied || m.pipeline.ChaosRound >= maxChaosRounds {
			m.pipeline.CurrentStage = StageAporia
			m.pipeline.InChaosRebuttal = false
			return `{"stage_accepted": "ANTITHESIS_EVALUATE", "next_stage": "APORIA", "directive": "You are the Aporia Engine, the final synergizer. Review the ENTIRE dialectic including the Chaos stress test and its rebuttal natively in your thought block. Formulate a comprehensive final synthesis that resolves all contradictions. You MUST provide BOTH aporia_synthesis (your full synthesis — not constrained to two sentences) AND synthesis_critique (explicit self-evaluation of what your synthesis handles well, handles poorly, and what assumptions remain). If synthesis is impossible, set paradox_detected=true and provide a resolution_strategy."}`, nil
		}

		// Not satisfied and more chaos rounds available → loop back to CHAOS
		m.pipeline.CurrentStage = StageChaos
		m.pipeline.InChaosRebuttal = false
		return fmt.Sprintf(`{"stage_accepted": "ANTITHESIS_EVALUATE", "next_stage": "CHAOS", "directive": "You are the Chaos Architect. The previous Black Swan defense was found INSUFFICIENT. The dialectic has consumed %d bytes across %d rounds. Review the original problem: [%s]. You MUST introduce a NEW, DISTINCT 'Black Swan' that: (1) is grounded in the ORIGINAL PROBLEM, not abstract philosophy, (2) targets a DIFFERENT assumption or gap than the previous chaos challenge, (3) would cause material failure if unaddressed. Distill into exactly TWO SENTENCES: the destabilizing event or scenario, then the specific mechanism by which it breaks the consensus."}`,
			m.pipeline.ContextBytes, m.pipeline.DialecticRound, m.pipeline.OriginalPrompt), nil
	}

	// --- Pre-Chaos Dialectic Phase ---

	// Absolute max: force convergence at maxDialecticRounds regardless
	if m.pipeline.DialecticRound >= maxDialecticRounds {
		m.pipeline.CurrentStage = StageChaos
		return m.buildChaosTransition(), nil
	}

	// Minimum enforcement: override premature convergence before minimum rounds
	if isSatisfied && m.pipeline.DialecticRound < minDialecticRounds {
		m.pipeline.DialecticRound++
		m.pipeline.CurrentStage = StageThesisDefense
		return fmt.Sprintf(`{"stage_accepted": "ANTITHESIS_EVALUATE", "convergence_rejected": true, "next_stage": "THESIS_DEFENSE", "directive": "CONVERGENCE REJECTED: Your satisfaction is premature — only %d of minimum %d rounds completed. The dialectic has not been sufficiently stress-tested. You MUST probe deeper. Specifically: (1) identify at least ONE assumption you haven't challenged, (2) find at least ONE edge case or failure mode not yet discussed, (3) ask the 'Unasked Question' — what critical aspect of the problem has neither the Thesis nor Antithesis addressed? Distill into exactly TWO SENTENCES: your position, then the specific technical evidence supporting it. Call with stage=THESIS_DEFENSE."}`,
			m.pipeline.DialecticRound-1, minDialecticRounds), nil
	}

	// Normal convergence: agent satisfied after minimum rounds met
	if isSatisfied {
		m.pipeline.CurrentStage = StageChaos
		return m.buildChaosTransition(), nil
	}

	// Continue dialectic: agent not satisfied
	m.pipeline.DialecticRound++
	m.pipeline.CurrentStage = StageThesisDefense

	// Round-indexed defense escalation
	switch {
	case m.pipeline.DialecticRound <= 2:
		// Round 2: Accountability — demand evidence-backed rebuttals
		return `{"stage_accepted": "ANTITHESIS_EVALUATE", "next_stage": "THESIS_DEFENSE", "directive": "You are the Thesis Architect. You must defend with INCREASED ACCOUNTABILITY natively in your thought block. You MUST explicitly identify which prior challenges remain UNADDRESSED or were only partially resolved. For each rebuttal, provide a concrete technical artifact, metric, or mechanism — not just reasoning. Concessions without evidence are FORBIDDEN. Distill into exactly TWO SENTENCES: your defense, then the specific technical artifact, metric, or mechanism that validates it. Call with stage=THESIS_DEFENSE."}`, nil
	default:
		// Round 3+: Steelman + Red Team — argue against your own position
		return `{"stage_accepted": "ANTITHESIS_EVALUATE", "next_stage": "THESIS_DEFENSE", "directive": "You are the Thesis Architect. Apply STEELMAN + RED TEAM methodology natively in your thought block. You MUST present the STRONGEST possible argument AGAINST your own position (Steelman the Antithesis), then demonstrate with specific evidence why your position survives it despite this strongest counterargument. If you cannot Steelman the opposing view, your position is insufficiently rigorous. Distill into exactly TWO SENTENCES: your defense, then the specific technical artifact, metric, or mechanism that validates it. Call with stage=THESIS_DEFENSE."}`, nil
	}
}

// buildChaosTransition constructs the Chaos Architect directive with the full
// stage-tagged transcript and original problem injection.
func (m *Machine) buildChaosTransition() string {
	transcriptStr := m.buildTranscriptStr()
	return fmt.Sprintf(`{"stage_accepted": "ANTITHESIS_EVALUATE", "next_stage": "CHAOS", "directive": "You are the Chaos Architect. The Thesis and Antithesis have reached an agreement within their shared frame of reference after %d rounds consuming %d bytes. Review the original problem: [%s]. Review the consensus trail: [%s]. COVERAGE ADVISORY: The dialectic challenged across these categories: Correctness, Security, Scalability, Edge Cases, Unasked Questions, and Dimensional Omissions. Your Black Swan MUST target a specific assumption or gap OUTSIDE the dimensions already addressed in the consensus trail to maximize destabilization potential. Focus on operational blind spots, failure modes under real-world conditions, or systemic risks that the Thesis/Antithesis frame of reference structurally cannot self-identify. You MUST introduce a 'Black Swan' that: (1) is grounded in the ORIGINAL PROBLEM, not abstract philosophy, (2) targets a specific assumption or gap in the consensus, (3) would cause material failure if unaddressed. Distill into exactly TWO SENTENCES: the destabilizing event or scenario, then the specific mechanism by which it breaks the consensus. Call with stage=CHAOS."}`,
		m.pipeline.DialecticRound, m.pipeline.ContextBytes, m.pipeline.OriginalPrompt, transcriptStr)
}

// buildTranscriptStr formats the LemmaTrail as a stage-tagged transcript string.
func (m *Machine) buildTranscriptStr() string {
	parts := make([]string, 0, len(m.pipeline.LemmaTrail))
	for _, entry := range m.pipeline.LemmaTrail {
		parts = append(parts, fmt.Sprintf("[%s·R%d] %s", entry.Stage, entry.Round, entry.Text))
	}
	return strings.Join(parts, " → ")
}

func (m *Machine) handleChaos(lemma string) (string, error) {
	if strings.TrimSpace(lemma) == "" {
		return m.formatError("CHAOS"), errors.New("missing lemma")
	}

	m.pipeline.LemmaTrail = append(m.pipeline.LemmaTrail, LemmaEntry{
		Stage: StageChaos,
		Round: m.pipeline.ChaosRound + 1,
		Text:  strings.TrimSpace(lemma),
	})
	m.pipeline.InChaosRebuttal = true
	m.pipeline.ChaosRound++
	m.pipeline.CurrentStage = StageThesisDefense

	return fmt.Sprintf(`{"stage_accepted": "CHAOS", "next_stage": "THESIS_DEFENSE", "directive": "You are the Thesis Architect. The Chaos Architect has introduced a Black Swan event that destabilizes the consensus. You MUST evaluate whether your original thesis survives this destabilization natively in your thought block. Reference the ORIGINAL PROBLEM: [%s]. Context consumed: %d bytes. Identify what breaks and what holds. Distill into exactly TWO SENTENCES: what survives the Black Swan, then what specific component or assumption breaks under it. Call with stage=THESIS_DEFENSE."}`,
		m.pipeline.OriginalPrompt, m.pipeline.ContextBytes), nil
}

func (m *Machine) handleAporia(req Request) (string, error) {
	if req.ParadoxDetected {
		m.pipeline.DeadlockCount++
		return `{"status": "paradox_detected", "directive": "Aporia failed. Apply your resolution_strategy to break the deadlock and attempt synthesis again natively. Then call APORIA again with aporia_synthesis, synthesis_critique, and paradox_detected=false."}`, nil
	}

	if strings.TrimSpace(req.AporiaSynthesis) == "" {
		return m.formatError("APORIA"), errors.New("missing aporia_synthesis")
	}

	// Enforce synthesis_critique — the final quality gate
	if strings.TrimSpace(req.SynthesisCritique) == "" {
		return `{"stage_accepted": "APORIA", "synthesis_rejected": true, "directive": "Your synthesis lacks self-critique. Before finalizing, you MUST provide a synthesis_critique that explicitly evaluates: (1) what your synthesis handles well, (2) what it handles poorly or incompletely, (3) any remaining assumptions or risks. Then resubmit APORIA with BOTH aporia_synthesis and synthesis_critique."}`, nil
	}

	return m.MuzzleAndSynthesize(req.AporiaSynthesis), nil
}

// MuzzleAndSynthesize builds the final squeezed output format with stage-tagged lemma trail.
func (m *Machine) MuzzleAndSynthesize(aporia string) string {
	var out strings.Builder
	out.WriteString("Socratic pipeline complete. Please present the following final synthesized solution EXACTLY AS IS to the user to maintain the optimal context window UI folding. Do not call the tool again for this session.\n\n")

	out.WriteString("### 🛤️ Dialectic Lemma Trail\n")
	for i, entry := range m.pipeline.LemmaTrail {
		label := fmt.Sprintf("%s·R%d", entry.Stage, entry.Round)
		out.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, label, entry.Text))
	}
	out.WriteString("\n")

	out.WriteString("### ⚖️ Aporia Verdict\n")
	out.WriteString(strings.TrimSpace(aporia))

	// Implicitly reset for next run, but preserve metrics for the dashboard
	m.pipeline = &PipelineState{
		CurrentStage:   StageIdle,
		DialecticRound: m.pipeline.DialecticRound,
		ChaosRound:     m.pipeline.ChaosRound,
		ContextBytes:   m.pipeline.ContextBytes,
		TokensEst:      m.pipeline.TokensEst,
	}

	return out.String()
}

// GetMetrics returns telemetry metrics for the state machine.
// It uses TryLock to avoid blocking the emission goroutine when
// Process() holds the mutex during active pipeline stages.
func (m *Machine) GetMetrics() (string, int, int, int) {
	if m.mu.TryLock() {
		m.lastMetrics = cachedMetrics{
			stage:           string(m.pipeline.CurrentStage),
			trifectaReviews: m.pipeline.DialecticRound + m.pipeline.ChaosRound,
			contextBytes:    m.pipeline.ContextBytes,
			tokensEst:       m.pipeline.TokensEst,
		}
		m.mu.Unlock()
	}
	// If TryLock failed, return cached values from last successful read.
	// This provides stale-but-valid data instead of blocking the emission goroutine.
	c := m.lastMetrics
	return c.stage, c.trifectaReviews, c.contextBytes, c.tokensEst
}

// formatError provides explicit instruction to the agent if they mess up the format, breaking infinite silent loops.
func (m *Machine) formatError(expectedStage string) string {
	return fmt.Sprintf("Error: Expected stage '%s', but received an invalid tool payload. "+
		"Please check the JSON schema and ensure you provided the correct fields. If you wish to restart, "+
		"submit 'RESET' as the stage.", expectedStage)
}
