package socratic_test

import (
	"context"
	"strings"
	"testing"

	"mcp-server-socratic-thinker/internal/socratic"
)

func TestMachine_Process_HappyPath(t *testing.T) {
	m := socratic.NewMachine()
	ctx := context.Background()

	res, err := m.Process(ctx, socratic.Request{Stage: "INITIALIZE", Problem: "What is testing?"})
	if err != nil {
		t.Fatalf("INITIALIZE failed: %v", err)
	}
	if !strings.Contains(res, "Thesis Architect") {
		t.Errorf("Unexpected INITIALIZE result: %s", res)
	}

	res, err = m.Process(ctx, socratic.Request{Stage: "THESIS", Lemma: "Testing is good."})
	if err != nil {
		t.Fatalf("THESIS failed: %v", err)
	}
	if !strings.Contains(res, "Antithesis Skeptic") {
		t.Errorf("Unexpected THESIS result: %s", res)
	}

	res, err = m.Process(ctx, socratic.Request{Stage: "ANTITHESIS_INITIAL", Lemma: "Testing takes time."})
	if err != nil {
		t.Fatalf("ANTITHESIS_INITIAL failed: %v", err)
	}
	if !strings.Contains(res, "Thesis Architect") {
		t.Errorf("Unexpected ANTITHESIS_INITIAL result: %s", res)
	}

	res, err = m.Process(ctx, socratic.Request{Stage: "THESIS_DEFENSE", Lemma: "Time is money, testing saves money."})
	if err != nil {
		t.Fatalf("THESIS_DEFENSE failed: %v", err)
	}
	if !strings.Contains(res, "Antithesis Skeptic") {
		t.Errorf("Unexpected THESIS_DEFENSE result: %s", res)
	}

	// First pass evaluate, is_satisfied = false, triggers Dialectic Loop
	res, err = m.Process(ctx, socratic.Request{Stage: "ANTITHESIS_EVALUATE", Lemma: "Prove it.", IsSatisfied: false})
	if err != nil {
		t.Fatalf("ANTITHESIS_EVALUATE (loop) failed: %v", err)
	}
	if !strings.Contains(res, "increasingly difficult") {
		t.Errorf("Unexpected ANTITHESIS_EVALUATE loop result: %s", res)
	}

	// Back to THESIS_DEFENSE
	res, err = m.Process(ctx, socratic.Request{Stage: "THESIS_DEFENSE", Lemma: "Bugs cost 10x in production."})
	if err != nil {
		t.Fatalf("THESIS_DEFENSE (loop) failed: %v", err)
	}

	// Second evaluate, is_satisfied = true, triggers CHAOS
	res, err = m.Process(ctx, socratic.Request{Stage: "ANTITHESIS_EVALUATE", Lemma: "Fair point.", IsSatisfied: true})
	if err != nil {
		t.Fatalf("ANTITHESIS_EVALUATE (satisfied) failed: %v", err)
	}
	if !strings.Contains(res, "Chaos Architect") {
		t.Errorf("Unexpected ANTITHESIS_EVALUATE satisfied result: %s", res)
	}

	res, err = m.Process(ctx, socratic.Request{Stage: "CHAOS", Lemma: "But what if we have no users?"})
	if err != nil {
		t.Fatalf("CHAOS failed: %v", err)
	}
	if !strings.Contains(res, "Aporia Engine") {
		t.Errorf("Unexpected CHAOS result: %s", res)
	}

	res, err = m.Process(ctx, socratic.Request{Stage: "APORIA", AporiaSynthesis: "Testing is only useful with users."})
	if err != nil {
		t.Fatalf("APORIA failed: %v", err)
	}
	if !strings.Contains(res, "Socratic pipeline complete") {
		t.Errorf("Unexpected APORIA result: %s", res)
	}
}

func TestMachine_Process_Reset(t *testing.T) {
	m := socratic.NewMachine()
	ctx := context.Background()

	m.Process(ctx, socratic.Request{Stage: "INITIALIZE", Problem: "prob"})
	res, err := m.Process(ctx, socratic.Request{Stage: "RESET"})
	if err != nil {
		t.Fatalf("RESET failed: %v", err)
	}
	if !strings.Contains(res, "Pipeline reset") {
		t.Errorf("Unexpected RESET result: %s", res)
	}

	_, err = m.Process(ctx, socratic.Request{Stage: "THESIS", Lemma: "foo"})
	if err == nil {
		t.Error("Expected error for THESIS when IDLE")
	}
}

func TestMachine_Process_InvalidState(t *testing.T) {
	m := socratic.NewMachine()
	ctx := context.Background()

	_, err := m.Process(ctx, socratic.Request{Stage: "THESIS", Lemma: "foo"})
	if err == nil || !strings.Contains(err.Error(), "invalid stage") {
		t.Errorf("Expected invalid stage error, got: %v", err)
	}

	m.Process(ctx, socratic.Request{Stage: "INITIALIZE", Problem: "prob"})
	
	_, err = m.Process(ctx, socratic.Request{Stage: "ANTITHESIS_INITIAL", Lemma: "foo"})
	if err == nil || !strings.Contains(err.Error(), "invalid stage") {
		t.Errorf("Expected invalid stage error, got: %v", err)
	}
}

func TestMachine_Process_MissingLemma(t *testing.T) {
	m := socratic.NewMachine()
	ctx := context.Background()

	m.Process(ctx, socratic.Request{Stage: "INITIALIZE", Problem: "prob"})
	
	_, err := m.Process(ctx, socratic.Request{Stage: "THESIS", Lemma: ""})
	if err == nil || !strings.Contains(err.Error(), "missing lemma") {
		t.Errorf("Expected missing lemma error for THESIS, got: %v", err)
	}

	m.Process(ctx, socratic.Request{Stage: "THESIS", Lemma: "t"})

	_, err = m.Process(ctx, socratic.Request{Stage: "ANTITHESIS_INITIAL", Lemma: ""})
	if err == nil || !strings.Contains(err.Error(), "missing lemma") {
		t.Errorf("Expected missing lemma error for ANTITHESIS_INITIAL, got: %v", err)
	}

	m.Process(ctx, socratic.Request{Stage: "ANTITHESIS_INITIAL", Lemma: "a"})

	_, err = m.Process(ctx, socratic.Request{Stage: "THESIS_DEFENSE", Lemma: ""})
	if err == nil || !strings.Contains(err.Error(), "missing lemma") {
		t.Errorf("Expected missing lemma error for THESIS_DEFENSE, got: %v", err)
	}

	m.Process(ctx, socratic.Request{Stage: "THESIS_DEFENSE", Lemma: "td"})

	_, err = m.Process(ctx, socratic.Request{Stage: "ANTITHESIS_EVALUATE", Lemma: "", IsSatisfied: true})
	if err == nil || !strings.Contains(err.Error(), "missing lemma") {
		t.Errorf("Expected missing lemma error for ANTITHESIS_EVALUATE, got: %v", err)
	}

	m.Process(ctx, socratic.Request{Stage: "ANTITHESIS_EVALUATE", Lemma: "ae", IsSatisfied: true})

	_, err = m.Process(ctx, socratic.Request{Stage: "CHAOS", Lemma: ""})
	if err == nil || !strings.Contains(err.Error(), "missing lemma") {
		t.Errorf("Expected missing lemma error for CHAOS, got: %v", err)
	}

	m.Process(ctx, socratic.Request{Stage: "CHAOS", Lemma: "c"})

	_, err = m.Process(ctx, socratic.Request{Stage: "APORIA", AporiaSynthesis: ""})
	if err == nil || !strings.Contains(err.Error(), "missing aporia_synthesis") {
		t.Errorf("Expected missing aporia_synthesis error, got: %v", err)
	}
}

func TestMachine_Process_ParadoxDetected(t *testing.T) {
	m := socratic.NewMachine()
	ctx := context.Background()

	m.Process(ctx, socratic.Request{Stage: "INITIALIZE", Problem: "prob"})
	m.Process(ctx, socratic.Request{Stage: "THESIS", Lemma: "t"})
	m.Process(ctx, socratic.Request{Stage: "ANTITHESIS_INITIAL", Lemma: "a"})
	m.Process(ctx, socratic.Request{Stage: "THESIS_DEFENSE", Lemma: "td"})
	m.Process(ctx, socratic.Request{Stage: "ANTITHESIS_EVALUATE", Lemma: "ae", IsSatisfied: true})
	m.Process(ctx, socratic.Request{Stage: "CHAOS", Lemma: "c"})

	res, err := m.Process(ctx, socratic.Request{Stage: "APORIA", ParadoxDetected: true})
	if err != nil {
		t.Fatalf("Paradox failed: %v", err)
	}
	if !strings.Contains(res, "paradox_detected") {
		t.Errorf("Expected paradox detected response, got: %s", res)
	}
}

func TestMachine_Process_Cancel(t *testing.T) {
	m := socratic.NewMachine()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // instantly cancel

	_, err := m.Process(ctx, socratic.Request{Stage: "INITIALIZE", Problem: "prob"})
	if err == nil || err != context.Canceled {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
}
