// Package pipeline provides functionality for the pipeline subsystem.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/staging"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
)

// AporiaEngineTool is the Moderator of Paradox. It synthesizes
// thesis_architect and antithesis_skeptic outputs to find Aporia
// points and determine the safe path forward.
type AporiaEngineTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

// Name performs the Name operation.
func (t *AporiaEngineTool) Name() string {
	return "aporia_engine"
}

// Register performs the Register operation.
func (t *AporiaEngineTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: SYNTHESIZER] MODERATOR OF PARADOX: Synthesizes thesis and antithesis arguments to find Aporia — points where both arguments are technically valid but mutually exclusive. Cross-references 6 pillar pairs to determine the safe path forward. [REQUIRES: brainstorm:thesis_architect, brainstorm:antithesis_skeptic] [Routing Tags: aporia, paradox, synthesis, resolve-conflict, merge-arguments]",
	}, t.Handle)
}

// AporiaInput is the typed input for the aporia_engine.
type AporiaInput struct {
	models.UniversalPipelineInput
}

// Handle performs the Handle operation.
func (t *AporiaEngineTool) Handle(ctx context.Context, _ *mcp.CallToolRequest, input AporiaInput) (*mcp.CallToolResult, any, error) {
	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("load session: %v", err))
		return res, nil, nil
	}

	report := models.AporiaReport{}

	// --- Phase 1: Load thesis and antithesis from session or context ---

	var thesis models.ThesisDocument
	var counter models.CounterThesisReport
	hasDialecticPair := false

	if input.Context != "" && staging.IsStagingURI(input.Context) && t.Engine != nil && t.Engine.DB != nil {
		var payload struct {
			Thesis  models.ThesisDocument      `json:"thesis"`
			Counter models.CounterThesisReport `json:"counter"`
		}
		if err := staging.LoadPayload(t.Engine.DB, input.Context, &payload); err == nil {
			thesis = payload.Thesis
			counter = payload.Counter
			if len(thesis.Data.Pillars) > 0 && len(counter.Pillars) > 0 {
				hasDialecticPair = true
			}
		} else {
			slog.Warn("[aporia_engine] failed to load explicitly staged dialetics", "uri", input.Context, "error", err)
		}
	}

	if !hasDialecticPair {
		if raw, ok := session.Metadata["thesis_document"]; ok {
			if doc, ok := raw.(models.ThesisDocument); ok {
				thesis = doc
			} else {
				// Handle JSON-deserialized map[string]interface{} from recall
				if b, err := json.Marshal(raw); err == nil {
					_ = json.Unmarshal(b, &thesis)
				}
			}
		}
	}

	if raw, ok := session.Metadata["counter_thesis"]; ok {
		if doc, ok := raw.(models.CounterThesisReport); ok {
			counter = doc
		} else {
			if b, err := json.Marshal(raw); err == nil {
				_ = json.Unmarshal(b, &counter)
			}
		}
	}

	if len(thesis.Data.Pillars) > 0 && len(counter.Pillars) > 0 {
		hasDialecticPair = true
	}

	// --- Phase 2: Cross-reference thesis/antithesis pillars ---

	if hasDialecticPair {
		resolutions := t.Engine.ResolveSafePath(ctx, thesis, counter)
		report.Resolutions = resolutions
		report.SafePathVerdict = engine.ComputeSafePathVerdict(resolutions)

		// Check for true paradoxes
		for _, r := range resolutions {
			if r.Resolution == "APORIA" {
				report.RefusalToProceed = true
				break
			}
		}
	}

	// --- Phase 3: Independent Socratic Vectors (always run) ---

	planText, _ := session.Metadata[state.KeyImplementationPlan].(string)
	if planText != "" {
		// Socratic Vector 1: Generic Bloat
		if strings.Contains(planText, "[T ") || strings.Contains(planText, "[any]") {
			report.GenericBloat = "DETECTED GENERIC BLOAT: Does this novel generic constraint actively prevent type erasure, or are you simply satisfying the Typeindex at the cost of compilation latency?"
			report.RefusalToProceed = true
		}

		// Socratic Vector 2: Zero-Value Trap
		if strings.Contains(planText, " *") && !strings.Contains(planText, "return nil") && !strings.Contains(planText, "== nil") {
			report.ZeroValueTrap = "DETECTED ZERO-VALUE TRAP: A pointer or interface is utilized without defensive nil bounds checking. Is this a micro-optimization or a future panic?"
			report.RefusalToProceed = true
		}

		// Socratic Vector 3: Green Tea Locality
		if strings.Contains(planText, "make([]") && strings.Contains(planText, "append(") {
			report.GreenTeaLocality = "GREEN TEA LOCALITY CHALLENGE: Aggressive slice allocation mapped! This natively bypasses Green Tea GC locality optimizations causing heap escapes."
			report.RefusalToProceed = true
		}
	}

	// --- Phase 4: STRIDE Threat Model (orchestrator-only) ---

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if recallAvailable {
		// Socratic Vector 4: STRIDE Threat Model
		analysisText := planText
		if thesis.Data.Narrative != "" {
			analysisText = thesis.Data.Narrative
		}
		if analysisText != "" {
			var traceMap map[string]any
			if session.ProjectRoot != "" {
				tm, err := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "go-refactor", session.ProjectRoot)
				if err == nil {
					traceMap = tm
				}
			}
			stride, err := t.Engine.AnalyzeThreatModel(ctx, analysisText, traceMap)
			if err == nil {
				tm := stride.Data.Metrics
				totalSTRIDE := tm.Spoofing + tm.Tampering + tm.Repudiation + tm.InformationLeak + tm.DenialOfService + tm.ElevationOfPrivilege
				if totalSTRIDE > 3 {
					report.RedTeamScore = totalSTRIDE
					report.RedTeamVerdict = "STRIDE: HIGH threat indicators"
					report.RefusalToProceed = true
				}
			}
		}

		// Standards integration
		_ = t.Engine.EnsureRecallCache(ctx, session, "aporia_engine", "search", map[string]any{"namespace": "ecosystem", "query": "Aporia Socratic Dialectics"})
	}

	// --- Phase 5: Publish results ---

	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["aporia_report"] = report

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("save session: %v", err))
		return res, nil, nil
	}

	// Publish to recall for pipeline composability
	if recallAvailable && session.ProjectRoot != "" {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, session.ProjectRoot, report.SafePathVerdict, "native", "aporia_engine", "", map[string]any{
			"safe_path_verdict": report.SafePathVerdict,
			"refusal":           report.RefusalToProceed,
			"resolutions":       report.Resolutions,
			"phase":             "synthesis",
			"stage":             "aporia_engine",
			"last_tool":         "aporia_engine",
		})
	}

	// CSSA publishing
	if input.SessionID != "" && recallAvailable {
		_ = t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, report)
	}

	// BuntDB Socratic Verdict Staging
	if input.SessionID != "" && t.Engine != nil && t.Engine.DB != nil {
		_ = staging.SaveSocraticVerdict(t.Engine.DB, input.SessionID, t.Name(), report.SafePathVerdict, report)
	}

	return &mcp.CallToolResult{}, report, nil
}
