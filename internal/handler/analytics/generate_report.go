// Package analytics provides functionality for the analytics subsystem.
package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/staging"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
)

// GenerateReportTool consolidates session analytics into a Hybrid JSON
// artifact with dual-mode support (orchestrator vs standalone).
type GenerateReportTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

// Name performs the Name operation.
func (t *GenerateReportTool) Name() string {
	return "generate_final_report"
}

// Register performs the Register operation.
func (t *GenerateReportTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: REPORTING] [PHASE: TERMINAL] PIPELINE REPORT GENERATOR: Generates a consolidated Markdown artifact detailing the session's analytical outcomes with conflict resolution ('Rejects First' formatting). [REQUIRES: brainstorm:architectural_diagrammer] [Routing Tags: final-report, artifact, summarize-decisions, markdown-report]",
	}, t.Handle)
}

// GenerateReportInput defines the tool's input schema.
type GenerateReportInput struct {
	models.UniversalPipelineInput
}

// ReportMetadata carries provenance and verdict data in the Hybrid JSON envelope.
type ReportMetadata struct {
	SessionID   string `json:"session_id"`
	Mode        string `json:"mode"`
	Verdict     string `json:"verdict"`
	ServerID    string `json:"server_id"`
	GeneratedAt string `json:"generated_at"`
}

// HybridReport is the top-level Hybrid JSON payload returned by the tool.
type HybridReport struct {
	Status          string         `json:"status"`
	Metadata        ReportMetadata `json:"metadata"`
	MarkdownPayload string         `json:"markdown_payload"`
}

// conflictKeywords are exact JSON value substrings that indicate failures.
// Quoted to prevent false positives from partial word matches like FAILOVER.
var conflictKeywords = []string{
	`"REJECT"`, `"REJECTED"`,
	`"FAIL"`, `"FAILED"`,
	`"CRITICAL"`,
}

// Handle performs the Handle operation.
func (t *GenerateReportTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input GenerateReportInput) (*mcp.CallToolResult, any, error) {
	// Early context guard.
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	if input.SessionID == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("session_id is strictly required"))
		return res, nil, nil
	}

	mode := "standalone"
	isOrchestrated := false
	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvail := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	slog.Info("generate_final_report: recall availability check",
		"engine_nil", t.Engine == nil,
		"client_nil", t.Engine == nil || t.Engine.ExternalClient == nil,
		"recall_enabled", recallAvail,
		"orchestrator_env", isOrchestrator,
		"session_id", input.SessionID,
	)
	if recallAvail {
		isOrchestrated = true
		mode = "consolidated"
	}
	slog.Info("generate_final_report: mode resolved", "mode", mode, "session_id", input.SessionID)

	// Load cross-server historical session data only in standalone mode.
	// In consolidated mode, the report uses ONLY the most recent pipeline run data.
	var historicalReports string
	if !isOrchestrated && t.Engine != nil {
		localSession, _ := t.Manager.LoadSession(ctx)
		if localSession != nil && localSession.ProjectRoot != "" {
			historicalReports = t.Engine.LoadCrossSessionFromRecall(ctx, "brainstorm", localSession.ProjectRoot)
		}
	}

	var sessionData map[string]any
	var localGaps []models.Gap

	if isOrchestrated {
		data, err := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "brainstorm", input.SessionID)
		if err != nil {
			slog.Warn("generate_final_report: recall session aggregation failed, falling back to standalone",
				"session_id", input.SessionID,
				"err", err,
				"hint", "if err is 'recall_key_not_found' verify stages wrote to recall; if 'recall_unreachable' check recall server health",
			)
			isOrchestrated = false
			mode = "standalone"
		} else {
			sessionData = data

			// Attempt to aggregate go-refactor data for the same project.
			// If go-refactor was also run, merge its output into the report.
			grData, grErr := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "go-refactor", input.SessionID)
			if grErr == nil && len(grData) > 0 {
				sessionData["_go-refactor"] = grData
				mode = "consolidated-combined"
				slog.Info("generate_final_report: merged go-refactor data into combined report",
					"go-refactor_keys", len(grData),
				)
			} else {
				slog.Info("generate_final_report: no go-refactor data found, single-server report",
					"err", grErr,
				)
			}

			// Idempotency DB lock check.
			overrideForce := false
			if val, ok := input.Flags["force"].(bool); ok {
				overrideForce = val
			}

			if !overrideForce {
				if generated, ok := sessionData["final_report_generated"].(bool); ok && generated {
					slog.Info("generate_final_report: idempotency lock active, aborting", "session_id", input.SessionID)
					res := &mcp.CallToolResult{}
					res.SetError(fmt.Errorf("report already consolidated by final server pipeline, use force=true to overwrite"))
					return res, nil, nil
				}
			}
		}
	}

	if !isOrchestrated {
		localSession, err := t.Manager.LoadSession(ctx)
		if err != nil {
			res := &mcp.CallToolResult{}
			res.SetError(fmt.Errorf("failed to load standalone session: %v", err))
			return res, nil, nil
		}

		sessionData = make(map[string]any)
		sessionData["project_root"] = localSession.ProjectRoot
		sessionData["project_name"] = localSession.ProjectName
		sessionData["language"] = localSession.Language
		sessionData["status"] = localSession.Status

		// Safely serialize metadata, skipping non-JSON-serializable values.
		sessionData["metadata"] = filterSerializable(localSession.Metadata)

		// Include gaps and history.
		localGaps = localSession.Gaps
		sessionData["gaps"] = localSession.Gaps
		sessionData["history"] = localSession.History

		slog.Info("generate_final_report: standalone session loaded",
			"project", localSession.ProjectRoot,
			"gaps", len(localSession.Gaps),
			"status", localSession.Status,
		)
	}

	// Enrich sessionData with historical session telemetry for the report narrative.
	if historicalReports != "" && sessionData != nil {
		// Cap historical reports to prevent report payload explosion.
		const maxHistory = 4096
		if len(historicalReports) > maxHistory {
			historicalReports = historicalReports[:maxHistory] + "\n...[truncated]"
		}
		sessionData["historical_sessions"] = historicalReports
	}

	// Create a pruned copy for serialization, excluding bulky aggregation data.
	// The full sessionData (with _stages) is still written to recall for idempotency.
	reportData := make(map[string]any, len(sessionData))
	for k, v := range sessionData {
		if k == "_stages" || k == "_stage_count" || k == "_total_entries" {
			continue // Skip internal aggregation metadata
		}
		reportData[k] = v
	}

	delete(reportData, "artifacts")
	if grClone, ok := reportData["_go-refactor"].(map[string]any); ok {
		delete(grClone, "artifacts")
		reportData["_go-refactor"] = grClone
	}

	// Conflict resolution ("Rejects First") sweep using quoted value matching.
	rawJson, _ := json.MarshalIndent(reportData, "", "  ")
	rawStr := string(rawJson)

	hasCriticalFlaw := false
	for _, keyword := range conflictKeywords {
		if strings.Contains(rawStr, keyword) {
			hasCriticalFlaw = true
			break
		}
	}

	verdict := "SUCCESS"
	if hasCriticalFlaw {
		verdict = "REJECTED"
	}
	slog.Info("generate_final_report: conflict resolution complete", "verdict", verdict)

	// GFM artifact generation.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Final Session Report [%s]\n\n", mode))

	if hasCriticalFlaw {
		sb.WriteString("> [!WARNING]\n> Critical flaws, failures, or rejections were identified during this pipeline workflow.\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("> [!NOTE]\n> Analytics session '%s' successfully evaluated with no adversarial flags.\n\n", input.SessionID))
	}

	// Structured gap rendering if gaps are available.
	gaps := localGaps
	if isOrchestrated {
		if g, ok := sessionData["gaps"]; ok {
			if gapBytes, err := json.Marshal(g); err == nil {
				var parsedGaps []models.Gap
				if json.Unmarshal(gapBytes, &parsedGaps) == nil {
					gaps = parsedGaps
				}
			}
		}
		// Angle 6: Merge cross-server metrics natively
		if gr, ok := sessionData["_go-refactor"].(map[string]any); ok {
			if g, ok := gr["gaps"]; ok {
				if gapBytes, err := json.Marshal(g); err == nil {
					var parsedGaps []models.Gap
					if json.Unmarshal(gapBytes, &parsedGaps) == nil {
						gaps = append(gaps, parsedGaps...)
					}
				}
			}
		}
	}

	if len(gaps) > 0 {
		sb.WriteString("## Analytical Findings\n\n")
		sb.WriteString("| Area | Description | Severity |\n")
		sb.WriteString("|------|-------------|----------|\n")
		for _, g := range gaps {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", g.Area, g.Description, g.Severity))
		}
		sb.WriteString("\n")
	}

	// CSSA Universal Artifact Builder
	var artifactCount int
	if arts, ok := sessionData["artifacts"].(map[string]any); ok {
		for name, content := range arts {
			artifactCount++
			if artifactCount == 1 {
				sb.WriteString("## Artifact Library\n\n")
			}
			sb.WriteString(fmt.Sprintf("### %s\n%v\n\n", name, content))
		}
	}
	if gr, ok := sessionData["_go-refactor"].(map[string]any); ok {
		if grArts, ok := gr["artifacts"].(map[string]any); ok {
			for name, content := range grArts {
				artifactCount++
				if artifactCount == 1 {
					sb.WriteString("## Artifact Library\n\n")
				}
				sb.WriteString(fmt.Sprintf("### %s\n%v\n\n", name, content))
			}
		}
	}

	// Load Socratic Verdicts from local BuntDB
	if t.Engine != nil && t.Engine.DB != nil {
		if verdicts, err := staging.LoadSocraticVerdicts(t.Engine.DB, input.SessionID); err == nil && len(verdicts) > 0 {
			sb.WriteString("## Stage-by-Stage Socratic Verdicts\n\n")
			sb.WriteString("| Stage / Tool | Verdict |\n")
			sb.WriteString("|--------------|---------|\n")
			for _, v := range verdicts {
				toolName, _ := v["tool"].(string)
				verdictStr, _ := v["verdict"].(string)
				sb.WriteString(fmt.Sprintf("| %s | `%s` |\n", toolName, verdictStr))
			}
			sb.WriteString("\n")
		} else if err != nil {
			slog.Warn("generate_final_report: failed to load socratic verdicts", "err", err)
		}
	}

	sb.WriteString("## Technical Delta\n\n```json\n")
	sb.WriteString(string(rawJson))
	sb.WriteString("\n```\n")

	// Idempotency lock write.
	if isOrchestrated {
		sessionData["final_report_generated"] = true
		if err := t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, sessionData); err != nil {
			slog.Error("generate_final_report: idempotency lock write failed", "session_id", input.SessionID, "err", err)
		} else {
			slog.Info("generate_final_report: idempotency lock persisted", "session_id", input.SessionID)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

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
		mdPath := filepath.Join(basePath, "brainstorm_final_report.md")
		_ = os.WriteFile(mdPath, []byte(content), 0644)
		
		jsonPath := mdPath + ".metadata.json"
		metaContent := `{"artifactType": "ARTIFACT_TYPE_WALKTHROUGH", "summary": "Final report automatically surfaced from pipeline telemetry.", "requestFeedback": false, "isArtifact": true}`
		_ = os.WriteFile(jsonPath, []byte(metaContent), 0644)
	}(reportContent, input.SessionID)

	report := HybridReport{
		Status: "success",
		Metadata: ReportMetadata{
			SessionID:   input.SessionID,
			Mode:        mode,
			Verdict:     verdict,
			ServerID:    "brainstorm",
			GeneratedAt: now,
		},
		MarkdownPayload: reportContent,
	}

	// Persist the report itself to recall for cross-server consumers.
	if isOrchestrated {
		if err := t.Engine.ExternalClient.SaveSession(ctx, input.SessionID+":final_report", input.SessionID, report); err != nil {
			slog.Warn("generate_final_report: failed to persist report artifact to recall", "err", err)
		}
	}

	// Publish report trace to recall sessions matrix for cross-session analytics.
	projectRoot, _ := sessionData["project_root"].(string)
	if projectRoot != "" {
		reportMeta := map[string]any{
			"verdict":   verdict,
			"mode":      mode,
			"gap_count": len(gaps),
		}
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, projectRoot, "report_generated", "native", "generate_final_report", "", reportMeta)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: sb.String(),
			},
		},
	}, report, nil
}

// deepPruneTelemetry recursively drops huge byte payloads to stop UI formatting bounds overflow.
func deepPruneTelemetry(node any) any {
	switch v := node.(type) {
	case map[string]any:
		for key, val := range v {
			lowKey := strings.ToLower(key)
			if strings.Contains(lowKey, "ast") || strings.Contains(lowKey, "syntax") || strings.Contains(lowKey, "source_code") {
				v[key] = "[UI Telemetry Pruned for Stability]"
				continue
			}
			v[key] = deepPruneTelemetry(val)
		}
		return v
	case []any:
		if len(v) > 25 {
			pruned := make([]any, 0, 26)
			for i := range 25 {
				pruned = append(pruned, deepPruneTelemetry(v[i]))
			}
			pruned = append(pruned, "[Array Truncated: Exceeds Safe Size]")
			return pruned
		}
		for i, val := range v {
			v[i] = deepPruneTelemetry(val)
		}
		return v
	case string:
		if len(v) > 2048 {
			return v[:2048] + "... [String Truncated for UI Rendering]"
		}
		return v
	default:
		return v
	}
}

// filterSerializable returns a copy of m with non-JSON-serializable values removed.
// Crucially executes a deep telemetry pruning pass mapping recursive AST elimination vectors.
func filterSerializable(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	safe := make(map[string]any, len(m))
	for k, v := range m {
		valStr := fmt.Sprintf("%v", v)
		if len(valStr) > 100000 {
			safe[k] = "[Unsafe Telemetry Object - Dropped via Massive Byte Count Limit]"
			continue
		}
		probe, err := json.Marshal(v)
		if err == nil && len(probe) > 0 {
			var generic any
			if err := json.Unmarshal(probe, &generic); err == nil {
				safe[k] = deepPruneTelemetry(generic)
			} else {
				safe[k] = v
			}
		}
	}
	return safe
}
