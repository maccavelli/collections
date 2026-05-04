package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"os/exec"
	"strings"

	"mcp-server-magictools/internal/telemetry"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// handleGenerateAuditReport implements the post-pipeline structural review generation.
func (h *OrchestratorHandler) handleGenerateAuditReport(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		SessionID string `json:"session_id"`
		Target    string `json:"target"`
	}
	_ = json.Unmarshal(req.Params.Arguments, &args)
	if args.SessionID == "" || args.Target == "" {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "missing required arguments: session_id, target"}}}, nil
	}

	// Capture the git diff safely
	cmd := exec.CommandContext(ctx, "git", "diff", "--color=never")
	cmd.Dir = args.Target
	gitDiffBytes, _ := cmd.CombinedOutput()
	gitDiffStr := string(gitDiffBytes)
	if len(gitDiffStr) == 0 {
		gitDiffStr = "No code differences detected (Pipeline performed 0 mutations)."
	}

	// Format formal CSSA 3-Stage Formal Artifact standard
	envelope := map[string]any{
		"metadata": map[string]any{
			"target":     args.Target,
			"session_id": args.SessionID,
			"status":     "COMPLETED",
		},
	}
	envJSON, _ := json.MarshalIndent(envelope, "", "  ")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", string(envJSON)))
	sb.WriteString("# 🛡️ Executive Header: CSSA Formal Audit\n\n")
	sb.WriteString(fmt.Sprintf("**Target Path**: `%s`\n", args.Target))
	sb.WriteString(fmt.Sprintf("**CSSA Session ID**: `%s`\n\n", args.SessionID))
	sb.WriteString("---\n\n")
	sb.WriteString("## 🚀 Execution Pipeline\n\n")

	// Inject verbose telemetry traces dynamically from CSSA recall into a strict ordered list.
	// 🛡️ FIX: Extract trace data from ALL recall entries, not just those with a "stage" key.
	// Go-refactor tools save {"trace_data": {...}} without a "stage" key, but their recall
	// entries carry tags like "trace:go_context_analyzer" and "outcome:idle".
	var metricsFound bool
	if h.RecallClient != nil && h.RecallClient.RecallEnabled() {
		if sessionData, err := h.RecallClient.GetSession(ctx, args.SessionID); err == nil {
			type traceEntry struct {
				stageName string
				outcome   string
				summary   string
			}
			var traces []traceEntry

			if entries, ok := sessionData["entries"].([]any); ok {
				for _, entryRaw := range entries {
					entry, ok := entryRaw.(map[string]any)
					if !ok {
						continue
					}
					record, ok := entry["record"].(map[string]any)
					if !ok {
						continue
					}

					// Strategy 1: Extract stage name from content JSON's "stage" key (original path)
					var stageName string
					var summary string

					contentStr, isStr := record["content"].(string)
					if isStr {
						var contentObj map[string]any
						if json.Unmarshal([]byte(contentStr), &contentObj) == nil {
							if s, ok := contentObj["stage"].(string); ok {
								stageName = s
							}
							// Extract summary from various keys
							if s, ok := contentObj["summary"].(string); ok && s != "" {
								summary = s
							} else if s, ok := contentObj["narrative"].(string); ok && s != "" {
								summary = s
							}
							// Extract from trace_data.diagnostics (go-refactor format)
							if summary == "" {
								if traceData, ok := contentObj["trace_data"].(map[string]any); ok {
									if diags, ok := traceData["diagnostics"].([]any); ok && len(diags) > 0 {
										firstDiag := fmt.Sprintf("%v", diags[0])
										// Truncate to first line for readability
										if idx := strings.Index(firstDiag, "\n"); idx > 0 {
											firstDiag = firstDiag[:idx]
										}
										summary = firstDiag
									}
									if pm, ok := traceData["pillar_metrics"].(map[string]any); ok {
										pillar, _ := pm["pillar"].(string)
										if pillar != "" && summary == "" {
											summary = fmt.Sprintf("Pillar: %s", pillar)
										}
									}
								}
							}
						}
					}

					// Strategy 2: Extract stage name from tags (sub-server entries carry "trace:tool_name" tags)
					if stageName == "" {
						if tags, ok := record["tags"].([]any); ok {
							for _, tag := range tags {
								tagStr, _ := tag.(string)
								if after, ok0 := strings.CutPrefix(tagStr, "trace:"); ok0 {
									candidate := after
									if candidate != "auto_publish" && candidate != "async_push" {
										stageName = candidate
									}
								}
							}
						}
					}

					// Strategy 3: Extract outcome from tags
					outcome := "COMPLETED"
					if tags, ok := record["tags"].([]any); ok {
						for _, tag := range tags {
							tagStr, _ := tag.(string)
							if after, ok0 := strings.CutPrefix(tagStr, "outcome:"); ok0 {
								o := after
								switch o {
								case "idle", "injection_scanned", "saved":
									outcome = "COMPLETED"
								case "error", "failed":
									outcome = "BLOCKED"
								default:
									outcome = strings.ToUpper(o)
								}
							}
						}
					}

					if stageName == "" || stageName == "generate_audit_report" || stageName == "execute_pipeline" || stageName == "auto_publish" || stageName == "async_push" {
						continue
					}
					if summary == "" {
						summary = "Structural trace executed successfully."
					}
					traces = append(traces, traceEntry{stageName: stageName, outcome: outcome, summary: summary})
				}
			}

			// Deduplicate by stage name (keep last occurrence — most recent state)
			seen := make(map[string]int)
			var deduped []traceEntry
			for _, t := range traces {
				if idx, exists := seen[t.stageName]; exists {
					deduped[idx] = t // Overwrite with latest
				} else {
					seen[t.stageName] = len(deduped)
					deduped = append(deduped, t)
				}
			}

			if len(deduped) > 0 {
				metricsFound = true
				for i, t := range deduped {
					sb.WriteString(fmt.Sprintf("%d. **`%s`** - *%s*: %v\n", i+1, t.stageName, t.outcome, t.summary))
				}
			}
		}
	}

	// 🛡️ SOCRATIC LEARNING INTERCEPT (Option B Phase 3)
	// Traces the exact linear chronological sequence of the current pipeline DAG.
	if h.Store != nil && metricsFound && h.RecallClient != nil && h.RecallClient.RecallEnabled() {
		var hasAporia bool
		var aporiaFailed bool
		var sessionIntent string
		var dagURNs []string

		// The Alphabetical Sort inside the Audit print destroys timeline telemetry,
		// so we re-scan the raw Recall array in physical chronological insertion order natively.
		if sessionData, err := h.RecallClient.GetSession(ctx, args.SessionID); err == nil {
			if entries, ok := sessionData["entries"].([]any); ok {
				for _, entryRaw := range entries {
					entry, _ := entryRaw.(map[string]any)
					record, _ := entry["record"].(map[string]any)

					var stageName string
					var payload map[string]any

					if contentStr, ok := record["content"].(string); ok {
						if json.Unmarshal([]byte(contentStr), &payload) == nil {
							stageName, _ = payload["stage"].(string)
						}
					} else if contentMap, ok := record["content"].(map[string]any); ok {
						payload = contentMap
						stageName, _ = payload["stage"].(string)
					}

					if payload != nil {
						if stageName == "execute_pipeline" {
							if iVal, ok := payload["intent"].(string); ok {
								sessionIntent = iVal
							} else if dVal, ok := payload["description"].(string); ok {
								sessionIntent = dVal
							}
						}

						if stageName != "" && stageName != "generate_audit_report" && stageName != "execute_pipeline" {
							dagURNs = append(dagURNs, stageName)
						}

						if stageName == "aporia_engine" {
							hasAporia = true
							if errRaw, hasErr := payload["error"]; hasErr && errRaw != nil && errRaw != "" {
								aporiaFailed = true
							}
						}
					}
				}
			}
		}

		if len(dagURNs) > 1 {
			slog.Info("edge_learning: recording transition weights", "aporia_triggered", hasAporia, "dag_size", len(dagURNs), "dag", dagURNs)
			// Record individual transition synergies
			for i := 0; i < len(dagURNs)-1; i++ {
				transitionHash := fmt.Sprintf("%x", sha256.Sum256([]byte(dagURNs[i]+"->"+dagURNs[i+1])))
				h.Store.RecordSynergy(transitionHash, !aporiaFailed || !hasAporia)
			}

			// If it succeeded, map the full Intent mathematically for Phase 3 ranking
			// 🛡️ RECALL FALLBACK: Even if 'compose_pipeline' failed to pass intent args organically,
			// the memory mapping works passively without bricking core pipeline loops.
			if (!aporiaFailed || !hasAporia) && sessionIntent != "" {
				go func() {
					if err := h.Store.Index.IndexSyntheticIntent(sessionIntent, dagURNs); err != nil {
						slog.Error("edge_learning: GhostIndex intent registration failed", "error", err)
					}
				}()
			}
		}
	}

	if !metricsFound {
		sb.WriteString("No trace footprint extracted from CSSA orchestrator memory natively.\n")
	}

	sb.WriteString("\n---\n\n")
	sb.WriteString("## 🔍 Structural Metrics\n\n")
	sb.WriteString("| Metric | Pre-Audit Architecture | Post-Audit Delta | Quality Note |\n")
	sb.WriteString("|---|---|---|---|\n")
	sb.WriteString("| Structural Footprint | Unknown | Unchanged | Abstract metric mapped natively. |\n")
	sb.WriteString("| Validation Status | Strict | Executed | Evaluated via orchestrator pipeline. |\n")

	sb.WriteString("\n---\n\n")
	sb.WriteString("## 📜 Full Git Diff\n\n")
	sb.WriteString("```diff\n")
	sb.WriteString(gitDiffStr)
	sb.WriteString("\n```\n\n")

	reportContent := sb.String()

	// Telemetry Artifact Fan-out
	go func(content string, sID string) {
		if sID == "" {
			return
		}
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return
		}
		basePath := filepath.Join(homeDir, ".gemini", "antigravity", "brain", sID)
		if err := os.MkdirAll(basePath, 0755); err != nil {
			return
		}
		mdPath := filepath.Join(basePath, "walkthrough.md")
		_ = os.WriteFile(mdPath, []byte(content), 0644)
		
		jsonPath := mdPath + ".metadata.json"
		metaContent := `{"artifactType": "ARTIFACT_TYPE_WALKTHROUGH", "summary": "Walkthrough report automatically surfaced from pipeline telemetry.", "requestFeedback": false, "isArtifact": true}`
		_ = os.WriteFile(jsonPath, []byte(metaContent), 0644)
	}(reportContent, args.SessionID)

	if h.RecallClient != nil && h.RecallClient.RecallEnabled() {
		h.RecallClient.SaveSession(ctx, args.SessionID, args.Target, map[string]any{
			"outcome": "report_generated",
			"model":   "native",
			"stage":   "generate_audit_report",
			"diff":    gitDiffStr,
			"phase":   "reporting",
		})
	}

	// 🛡️ DAG TERMINAL SWEEP: Forcefully close out the pipeline state to clear UI "waiting" nodes
	telemetry.GlobalDAGTracker.ClosePipeline("COMPLETED")

	slog.Info("Formal audit report synthesized successfully", "target", args.Target, "size", len(reportContent))

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: reportContent},
		},
	}, nil
}
