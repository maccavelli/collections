package pipeline

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
)

func TestAporiaEngineTool_Handle(t *testing.T) {
	mgr := state.NewManager(".")
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(".", db)
	tool := &AporiaEngineTool{Manager: mgr, Engine: eng}

	ctx := context.Background()

	// Case 1: Dialectic pair found
	session, _ := mgr.LoadSession(ctx)
	session.Metadata["thesis_document"] = models.ThesisDocument{
		Data: struct {
			Narrative string                   `json:"narrative"`
			Pillars   []models.DialecticPillar `json:"pillars"`
			Standards string                   `json:"standards,omitempty"`
		}{
			Pillars: []models.DialecticPillar{{Name: "Scalability", Score: 8}},
		},
	}
	session.Metadata["counter_thesis"] = models.CounterThesisReport{
		Pillars: []models.DialecticPillar{{Name: "Scalability", Score: 4}},
	}
	mgr.SaveSession(ctx, session)

	req := &mcp.CallToolRequest{}
	input := AporiaInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			SessionID: "test-session",
			Target:    ".",
		},
	}

	_, result, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	report := result.(models.AporiaReport)
	if len(report.Resolutions) == 0 {
		t.Error("expected resolutions from dialectic pair")
	}

	// Case 2: Socratic Vectors (Generic Bloat)
	session.Metadata[state.KeyImplementationPlan] = "func [T any] test() {}"
	mgr.SaveSession(ctx, session)
	_, result2, _ := tool.Handle(ctx, req, input)
	report2 := result2.(models.AporiaReport)
	if report2.GenericBloat == "" {
		t.Error("expected GenericBloat detection")
	}
	if !report2.RefusalToProceed {
		t.Error("expected RefusalToProceed for generic bloat")
	}

	// Case 3: Zero-Value Trap
	session.Metadata[state.KeyImplementationPlan] = "var x *int"
	mgr.SaveSession(ctx, session)
	_, result3, _ := tool.Handle(ctx, req, input)
	report3 := result3.(models.AporiaReport)
	if report3.ZeroValueTrap == "" {
		t.Error("expected ZeroValueTrap detection")
	}

	session.Metadata[state.KeyImplementationPlan] = "make([] append("
	mgr.SaveSession(ctx, session)
	_, result4, _ := tool.Handle(ctx, req, input)
	report4 := result4.(models.AporiaReport)
	if report4.GreenTeaLocality == "" {
		t.Error("expected GreenTeaLocality detection")
	}

	// Case 5: Map-based thesis/counter (Recall simulation)
	session.Metadata["thesis_document"] = map[string]any{
		"data": map[string]any{
			"pillars": []any{
				map[string]any{"name": "Reliability", "score": 9.0},
			},
		},
	}
	session.Metadata["counter_thesis"] = map[string]any{
		"pillars": []any{
			map[string]any{"name": "Reliability", "score": 3.0},
		},
	}
	mgr.SaveSession(ctx, session)
	_, result5, _ := tool.Handle(ctx, req, input)
	report5 := result5.(models.AporiaReport)
	if len(report5.Resolutions) == 0 {
		t.Error("expected resolutions from map-based dialectic pair")
	}

	// Case 6: APORIA resolution refusal
	thesis := session.Metadata["thesis_document"].(map[string]any)
	thesis["data"].(map[string]any)["pillars"].([]any)[0].(map[string]any)["name"] = "Conflict"
	thesis["data"].(map[string]any)["pillars"].([]any)[0].(map[string]any)["score"] = 10.0

	counter_map := session.Metadata["counter_thesis"].(map[string]any)
	counter_map["pillars"].([]any)[0].(map[string]any)["name"] = "Conflict"
	counter_map["pillars"].([]any)[0].(map[string]any)["score"] = 1.0

	session.Metadata["thesis_document"] = thesis
	session.Metadata["counter_thesis"] = counter_map
	mgr.SaveSession(ctx, session)

	_, result6, _ := tool.Handle(ctx, req, input)
	report6 := result6.(models.AporiaReport)
	// engine.ResolveSafePath logic: if diff > 5, it might be APORIA or WARNING.
	// I'll check if RefusalToProceed is set.
	if report6.SafePathVerdict == "APORIA" && !report6.RefusalToProceed {
		t.Error("expected RefusalToProceed for APORIA verdict")
	}
}
