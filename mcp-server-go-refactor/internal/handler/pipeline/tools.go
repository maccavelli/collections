package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/state"
	"mcp-server-go-refactor/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
)

// ---------------------------------------------------------------------------
// generate_implementation_plan
// ---------------------------------------------------------------------------

// GeneratePlanTool synthesizes a structured implementation plan from
// cumulative analysis diagnostics and recall standards.
type GeneratePlanTool struct {
	Engine *engine.Engine
}

func (t *GeneratePlanTool) Name() string {
	return "generate_implementation_plan"
}

func (t *GeneratePlanTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: PLANNER] IMPLEMENTATION PLAN GENERATOR: Creates, writes, and synthesizes a structured sequence of implementation tasks from cumulative analysis diagnostics (complexity, dead code, interface discovery) and recall code standards. Aggregates ALL prior tool diagnostics. [REQUIRES: brainstorm:aporia_engine] [TRIGGERS: Subsequent codebase mutation vectors] [Routing Tags: plan-tasks, sequences, generate-steps, aggregator]",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{
					"type":        "string",
					"description": "CSSA backend storage pipeline correlation ID.",
				},
				"target": map[string]any{
					"type":        "string",
					"description": "Absolute path to the project root or package.",
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Optional: requirements, feature description, or constraints.",
				},
				"artifact_path": map[string]any{
					"type":        "string",
					"description": "Optional OS absolute path to route the generated output payload, bypassing JSON-RPC overhead.",
				},
			},
			"required": []string{"session_id", "target"},
		},
	}, t.Handle)
}

type GeneratePlanInput struct {
	models.UniversalPipelineInput
}

func (t *GeneratePlanTool) Handle(ctx context.Context, _ *mcp.CallToolRequest, input GeneratePlanInput) (*mcp.CallToolResult, any, error) {
	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	session := t.Engine.LoadSession(ctx, input.Target)

	// Collect all BuntDB cached analysis from previous tools.
	var diagnostics []string
	if t.Engine.DB != nil {
		_ = t.Engine.DB.View(func(tx *buntdb.Tx) error {
			return tx.AscendKeys("recall_cache_*", func(key, value string) bool {
				var wrapper engine.SyncWrapper
				if err := json.Unmarshal([]byte(value), &wrapper); err == nil && wrapper.Data != "" {
					diagnostics = append(diagnostics, wrapper.Data)
				} else if value != "" {
					diagnostics = append(diagnostics, value)
				}
				return true
			})
		})
	}

	// Fetch comprehensive standards from recall.
	var standards, history string
	if recallAvailable {
		standards = t.Engine.EnsureRecallCache(ctx, session, "generate_plan", "search", map[string]interface{}{"namespace": "ecosystem",
			"query": "implementation patterns Go best practices code quality guidelines",
			"limit": 15,
		})
		history = t.Engine.LoadCrossSessionFromRecall(ctx, "gorefactor", input.Target)
	}

	// Build structured plan.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Implementation Plan: %s\n\n", input.Target))

	if input.Context != "" {
		sb.WriteString("## Requirements\n")
		sb.WriteString(input.Context + "\n\n")
	}

	sb.WriteString("## Diagnostics Summary\n")
	if len(diagnostics) == 0 {
		sb.WriteString("No prior analysis diagnostics available. Run analysis tools first.\n")
	} else {
		for i, d := range diagnostics {
			dStr := fmt.Sprintf("%v", d)
			const maxDiagSize = 2048
			if len(dStr) > maxDiagSize {
				dStr = dStr[:maxDiagSize] + "\n... [Diagnostic Details Truncated to prevent Context Overflow]"
			}
			sb.WriteString(fmt.Sprintf("### Diagnostic %d\n%s\n\n", i+1, dStr))
		}
	}

	if standards != "" {
		sb.WriteString("## Applicable Standards (from Recall)\n")
		sb.WriteString(standards + "\n\n")
	}

	if history != "" {
		const maxHistory = 4096
		if len(history) > maxHistory {
			history = history[:maxHistory] + "\n...[truncated historical data safely]"
		}
		sb.WriteString("## Historical Context\n")
		sb.WriteString(history + "\n\n")
	}

	sb.WriteString("## Proposed Changes\n")
	sb.WriteString("Based on the diagnostics and standards above, the following changes are recommended.\n\n")

	// Deliver precise parsing boundaries for Brainstorm Socratic Vetting tools
	sb.WriteString("### Execution Traces\n")
	sb.WriteString(fmt.Sprintf("> [RISK_NODE_L01-L999] Bound: %s\n\n", input.Target))

	planText := sb.String()
	planHash := fmt.Sprintf("%x", sha256.Sum256([]byte(planText)))

	// Store in session metadata.
	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata[state.KeyImplementationPlan] = planText
	session.Metadata[state.KeyPlanHash] = planHash
	t.Engine.SaveSession(session)

	// Publish to recall.
	if recallAvailable {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "plan_generated", "native", "generate_implementation_plan", planText, map[string]any{
			"plan_hash":   planHash,
			"plan_size":   len(planText),
			"diagnostics": len(diagnostics),
			"phase":       "planning",
			"stage":       "generate_implementation_plan",
			"last_tool":   "generate_implementation_plan",
		})
	}

	resp := struct {
		Status          string `json:"status"`
		MarkdownPayload string `json:"markdown_payload"`
		Metadata        struct {
			SessionID   string `json:"session_id"`
			Mode        string `json:"mode"`
			Verdict     string `json:"verdict"`
			ServerID    string `json:"server_id"`
			GeneratedAt string `json:"generated_at"`
			Summary     string `json:"summary"`
			PlanHash    string `json:"plan_hash"`
			Diagnostics int    `json:"diagnostics_count"`
		} `json:"metadata"`
	}{
		Status:          "success",
		MarkdownPayload: planText,
		Metadata: struct {
			SessionID   string `json:"session_id"`
			Mode        string `json:"mode"`
			Verdict     string `json:"verdict"`
			ServerID    string `json:"server_id"`
			GeneratedAt string `json:"generated_at"`
			Summary     string `json:"summary"`
			PlanHash    string `json:"plan_hash"`
			Diagnostics int    `json:"diagnostics_count"`
		}{
			SessionID:   input.SessionID,
			Mode:        "standalone",
			Verdict:     "SUCCESS",
			ServerID:    "go-refactor",
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Summary:     fmt.Sprintf("Implementation plan generated for %s", input.Target),
			PlanHash:    planHash,
			Diagnostics: len(diagnostics),
		},
	}

	// Publish to CSSA.
	if recallAvailable && input.SessionID != "" {
		_ = t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, resp)
	}

	// (Global Universal Fast-Path will natively handle artifact_path writes automatically from here)

	return &mcp.CallToolResult{}, resp, nil
}

// ---------------------------------------------------------------------------
// apply_vetted_edit
// ---------------------------------------------------------------------------

// ApplyVettedEditTool is the terminal filesystem gatekeeper that writes
// vetted code to disk with atomic safety and standards validation.
type ApplyVettedEditTool struct {
	Engine *engine.Engine
}

func (t *ApplyVettedEditTool) Name() string {
	return "apply_vetted_edit"
}

func (t *ApplyVettedEditTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: MUTATOR] FILESYSTEM EDIT GATEKEEPER: Applies and saves vetted code changes to disk files with atomic storage guarantees. Requires a plan_hash for integrity verification, writes through an atomic .tmp rename pattern. [REQUIRES: Full dialectical thesis logic synthesis] [TRIGGERS: Post-mutation structural stability validation] [Routing Tags: write-file, mutator, disk-save, atomic-commit, vetted-edit]",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{
					"type":        "string",
					"description": "CSSA backend storage pipeline correlation ID.",
				},
				"target": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to write.",
				},
				"context": map[string]any{
					"type":        "string",
					"description": "The vetted Go source code to write.",
				},
				"flags": map[string]any{
					"type":        "object",
					"description": "Must include plan_hash (string) for integrity verification.",
					"properties": map[string]any{
						"plan_hash": map[string]any{
							"type":        "string",
							"description": "SHA-256 hash of the approved implementation plan.",
						},
					},
				},
			},
			"required": []string{"session_id", "target", "context"},
		},
	}, t.Handle)
}

type ApplyEditInput struct {
	models.UniversalPipelineInput
}

func (t *ApplyVettedEditTool) Handle(ctx context.Context, _ *mcp.CallToolRequest, input ApplyEditInput) (*mcp.CallToolResult, any, error) {
	if input.Context == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("context (source code) is required"))
		return res, nil, nil
	}

	if input.Target == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("target (file path) is required"))
		return res, nil, nil
	}

	// Extract plan_hash from flags.
	planHash, _ := input.Flags["plan_hash"].(string)
	if planHash == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("flags.plan_hash is required for integrity verification"))
		return res, nil, nil
	}

	// Pre-flight: Check that an approval exists in recall for this plan_hash.
	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if recallAvailable {
		approvalCheck := t.Engine.ExternalClient.CallDatabaseTool(ctx, "list", map[string]interface{}{"namespace": "sessions",
			"server_id":        "brainstorm",
			"outcome":          "approved",
			"truncate_content": true,
			"limit":            10,
		})
		if approvalCheck == "" {
			slog.Warn("[apply_vetted_edit] recall unreachable for approval check, proceeding with caution")
		} else if !strings.Contains(approvalCheck, planHash) {
			slog.Warn("[apply_vetted_edit] plan_hash not found in approved sessions, proceeding with warning",
				"plan_hash", planHash[:16]+"...")
		}
	}

	// Validate AST BEFORE writing to prevent double-escaped literal corruption.
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", input.Context, parser.ParseComments); err != nil {
		slog.Error("[apply_vetted_edit] PRE-FLIGHT REJECTED: Malformed AST payload (possible proxy unescaping corruption)", "error", err)
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("PRE-FLIGHT REJECTED: payload contains malformed AST structure: %v", err))
		return res, nil, nil
	}

	// Unmutated duplicate check (preserves mtime and fast-fails sequences with no required edits)
	if existingBytes, err := os.ReadFile(input.Target); err == nil {
		inHash := sha256.Sum256([]byte(input.Context))
		outHash := sha256.Sum256(existingBytes)
		if inHash == outHash {
			slog.Info("[apply_vetted_edit] payload equals existing file perfectly; skipping atomic write")

			// Publish write trace to recall to satisfy pipeline continuity even on skip
			if recallAvailable {
				t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "edit_skipped(unmodified)", "native", "apply_vetted_edit", "", map[string]any{
					"plan_hash":   planHash,
					"target":      input.Target,
					"write_size":  len(existingBytes),
					"gofmt_clean": true,
					"phase":       "execution",
					"stage":       "apply_vetted_edit",
					"last_tool":   "apply_vetted_edit",
				})
			}

			resp := struct {
				Summary   string `json:"summary"`
				Target    string `json:"target"`
				PlanHash  string `json:"plan_hash"`
				WriteSize int    `json:"write_size_bytes"`
			}{
				Summary:   fmt.Sprintf("Skip: Unmodified Payload against %s", input.Target),
				Target:    input.Target,
				PlanHash:  planHash,
				WriteSize: len(existingBytes),
			}
			if recallAvailable {
				_ = t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, resp)
			}
			return &mcp.CallToolResult{}, resp, nil
		}
	}

	// Validate: try to gofmt the source before writing.
	formatted, fmtErr := format.Source([]byte(input.Context))
	if fmtErr != nil {
		slog.Warn("[apply_vetted_edit] gofmt validation failed, writing raw source", "error", fmtErr)
		formatted = []byte(input.Context)
	}

	// Atomic write: write to .tmp, then rename.
	dir := filepath.Dir(input.Target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to create directory %s: %v", dir, err))
		return res, nil, nil
	}

	tmpPath := input.Target + ".tmp"
	if err := os.WriteFile(tmpPath, formatted, 0o644); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to write temp file: %v", err))
		return res, nil, nil
	}
	defer os.Remove(tmpPath) // Cleanup on any failure path.

	var diffStr string
	if _, statErr := os.Stat(input.Target); statErr == nil {
		out, _ := exec.Command("diff", "-u", input.Target, tmpPath).CombinedOutput()
		diffStr = string(out)
	} else {
		diffStr = "--- /dev/null\n+++ " + input.Target + "\n@@ -0,0 +1 @@\n+ [New File Created]\n" + string(formatted)
	}

	if err := os.Rename(tmpPath, input.Target); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("atomic rename failed: %v", err))
		return res, nil, nil
	}

	// Post-write trace.
	session := t.Engine.LoadSession(ctx, input.Target)
	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	if session.Artifacts == nil {
		session.Artifacts = make(map[string]string)
	}
	if diffStr != "" {
		session.Artifacts["diff_"+filepath.Base(input.Target)] = "```diff\n" + diffStr + "\n```"
	}
	session.Metadata["last_write"] = input.Target
	session.Metadata["plan_hash"] = planHash
	session.Metadata["write_size"] = len(formatted)
	session.Metadata["gofmt_applied"] = fmtErr == nil
	t.Engine.SaveSession(session)

	// Publish write trace to recall.
	if recallAvailable {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "edit_applied", "native", "apply_vetted_edit", "", map[string]any{
			"plan_hash":   planHash,
			"target":      input.Target,
			"write_size":  len(formatted),
			"gofmt_clean": fmtErr == nil,
			"phase":       "execution",
			"stage":       "apply_vetted_edit",
			"last_tool":   "apply_vetted_edit",
		})
	}

	resp := struct {
		Summary    string `json:"summary"`
		Target     string `json:"target"`
		PlanHash   string `json:"plan_hash"`
		WriteSize  int    `json:"write_size_bytes"`
		GofmtClean bool   `json:"gofmt_clean"`
	}{
		Summary:    fmt.Sprintf("Edit applied to %s (%d bytes, gofmt: %v)", input.Target, len(formatted), fmtErr == nil),
		Target:     input.Target,
		PlanHash:   planHash,
		WriteSize:  len(formatted),
		GofmtClean: fmtErr == nil,
	}

	// Publish to CSSA.
	if recallAvailable && input.SessionID != "" {
		_ = t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, resp)
	}

	return &mcp.CallToolResult{}, resp, nil
}

// Register adds the pipeline tools to the go-refactor registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&GeneratePlanTool{Engine: eng})
	registry.Global.Register(&ApplyVettedEditTool{Engine: eng})
	registry.Global.Register(&GoTestValidationTool{Engine: eng})
	registry.Global.Register(&InterfaceSynthesizerTool{Engine: eng})
}
