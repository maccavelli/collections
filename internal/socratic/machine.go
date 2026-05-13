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
)

type PipelineState struct {
	OriginalPrompt string
	LemmaTrail     []string
	CurrentStage   Stage
	DeadlockCount  int
	DialecticRound int
	ContextBytes   int
	TokensEst      int
}

// Machine manages the single global pipeline.
type Machine struct {
	mu       sync.Mutex
	pipeline *PipelineState
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
		LemmaTrail:     []string{},
		DialecticRound: 0,
		ContextBytes:   0,
		TokensEst:      0,
	}
	return `{"stage_accepted": "INITIALIZE", "next_stage": "THESIS", "directive": "You are the Thesis Architect. Provide a clear, robust initial solution or hypothesis to the user's problem natively in your thought block. Once reached, distill it into a SINGLE-SENTENCE Lemma and call the tool with stage=THESIS."}`, nil
}

func (m *Machine) handleThesis(lemma string) (string, error) {
	if strings.TrimSpace(lemma) == "" {
		return m.formatError("THESIS"), errors.New("missing lemma")
	}

	m.pipeline.LemmaTrail = append(m.pipeline.LemmaTrail, strings.TrimSpace(lemma))
	m.pipeline.CurrentStage = StageAntithesisInitial
	m.pipeline.DialecticRound = 1

	return `{"stage_accepted": "THESIS", "next_stage": "ANTITHESIS_INITIAL", "directive": "You are the Antithesis Skeptic. Critique your previous Thesis natively in your thought block, identifying flaws, edge cases, and missing context. Generate n Challenge Questions. Distill your overarching critique into a SINGLE-SENTENCE Lemma and call the tool with stage=ANTITHESIS_INITIAL."}`, nil
}

func (m *Machine) handleAntithesisInitial(lemma string) (string, error) {
	if strings.TrimSpace(lemma) == "" {
		return m.formatError("ANTITHESIS_INITIAL"), errors.New("missing lemma")
	}

	m.pipeline.LemmaTrail = append(m.pipeline.LemmaTrail, strings.TrimSpace(lemma))
	m.pipeline.CurrentStage = StageThesisDefense

	return `{"stage_accepted": "ANTITHESIS_INITIAL", "next_stage": "THESIS_DEFENSE", "directive": "You are the Thesis Architect. Defend against the challenge questions natively in your thought block. Distill your defense into a SINGLE-SENTENCE Lemma and call the tool with stage=THESIS_DEFENSE."}`, nil
}

func (m *Machine) handleThesisDefense(lemma string) (string, error) {
	if strings.TrimSpace(lemma) == "" {
		return m.formatError("THESIS_DEFENSE"), errors.New("missing lemma")
	}

	m.pipeline.LemmaTrail = append(m.pipeline.LemmaTrail, strings.TrimSpace(lemma))
	m.pipeline.CurrentStage = StageAntithesisEvaluate

	return `{"stage_accepted": "THESIS_DEFENSE", "next_stage": "ANTITHESIS_EVALUATE", "directive": "You are the Antithesis Skeptic. Evaluate the defense natively in your thought block. If satisfied, call the tool with stage=ANTITHESIS_EVALUATE, is_satisfied=true, and a concluding Lemma. If unsatisfied, generate an 'Increasing Difficulty' prompt targeting Semantic Gaps and the 'Unasked Question', and call the tool with stage=ANTITHESIS_EVALUATE, is_satisfied=false, and a critique Lemma."}`, nil
}

func (m *Machine) handleAntithesisEvaluate(lemma string, isSatisfied bool) (string, error) {
	if strings.TrimSpace(lemma) == "" {
		return m.formatError("ANTITHESIS_EVALUATE"), errors.New("missing lemma")
	}

	m.pipeline.LemmaTrail = append(m.pipeline.LemmaTrail, strings.TrimSpace(lemma))

	if isSatisfied || m.pipeline.DialecticRound >= 3 {
		m.pipeline.CurrentStage = StageChaos
		transcriptStr := strings.Join(m.pipeline.LemmaTrail, " -> ")
		return fmt.Sprintf(`{"stage_accepted": "ANTITHESIS_EVALUATE", "next_stage": "CHAOS", "directive": "You are the Chaos Architect. The Thesis and Antithesis have reached an agreement within their shared frame of reference. Review their logical progression: [%s]. Use the 'Aporia' method to destabilize this consensus by introducing a 'Black Swan' event. Distill your destabilization into a SINGLE-SENTENCE Lemma and call the tool with stage=CHAOS."}`, transcriptStr), nil
	}

	m.pipeline.DialecticRound++
	m.pipeline.CurrentStage = StageThesisDefense
	return `{"stage_accepted": "ANTITHESIS_EVALUATE", "next_stage": "THESIS_DEFENSE", "directive": "You are the Thesis Architect. You must defend against the increasingly difficult critique natively in your thought block. Clarify definitions and resolve semantic gaps. Distill your defense into a SINGLE-SENTENCE Lemma and call the tool with stage=THESIS_DEFENSE."}`, nil
}

func (m *Machine) handleChaos(lemma string) (string, error) {
	if strings.TrimSpace(lemma) == "" {
		return m.formatError("CHAOS"), errors.New("missing lemma")
	}

	m.pipeline.LemmaTrail = append(m.pipeline.LemmaTrail, strings.TrimSpace(lemma))
	m.pipeline.CurrentStage = StageAporia

	return `{"stage_accepted": "CHAOS", "next_stage": "APORIA", "directive": "You are the Aporia Engine, the final synergizer. Review the entire dialectic and the Chaos event natively in your thought block. Formulate a final synthesis that resolves contradictions. Call the tool with stage=APORIA and aporia_synthesis. If synthesis is impossible, set paradox_detected=true and provide a resolution_strategy."}`, nil
}

func (m *Machine) handleAporia(req Request) (string, error) {
	if req.ParadoxDetected {
		m.pipeline.DeadlockCount++
		return `{"status": "paradox_detected", "directive": "Aporia failed. Apply your resolution_strategy to break the deadlock and attempt synthesis again natively. Then call APORIA again."}`, nil
	}

	if strings.TrimSpace(req.AporiaSynthesis) == "" {
		return m.formatError("APORIA"), errors.New("missing aporia_synthesis")
	}

	return m.MuzzleAndSynthesize(req.AporiaSynthesis), nil
}

// MuzzleAndSynthesize builds the final squeezed output format.
func (m *Machine) MuzzleAndSynthesize(aporia string) string {
	var out strings.Builder
	out.WriteString("Socratic pipeline complete. Please present the following final synthesized solution EXACTLY AS IS to the user to maintain the optimal context window UI folding. Do not call the tool again for this session.\n\n")

	out.WriteString("### 🛤️ Dialectic Lemma Trail\n")
	for i, l := range m.pipeline.LemmaTrail {
		out.WriteString(fmt.Sprintf("%d. %s\n", i+1, l))
	}
	out.WriteString("\n")

	out.WriteString("### ⚖️ Aporia Verdict\n")
	out.WriteString(strings.TrimSpace(aporia))

	// Implicitly reset for next run, but preserve metrics for the dashboard
	m.pipeline = &PipelineState{
		CurrentStage: StageIdle,
		ContextBytes: m.pipeline.ContextBytes,
		TokensEst:    m.pipeline.TokensEst,
	}

	return out.String()
}

// GetMetrics returns telemetry metrics for the state machine.
func (m *Machine) GetMetrics() (string, int, int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return string(m.pipeline.CurrentStage), m.pipeline.DeadlockCount, m.pipeline.ContextBytes, m.pipeline.TokensEst
}

// formatError provides explicit instruction to the agent if they mess up the format, breaking infinite silent loops.
func (m *Machine) formatError(expectedStage string) string {
	return fmt.Sprintf("Error: Expected stage '%s', but received an invalid tool payload. "+
		"Please check the JSON schema and ensure you provided the correct fields. If you wish to restart, "+
		"submit 'RESET' as the stage.", expectedStage)
}
