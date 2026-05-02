package design

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
)

// ArchitecturalDiagrammerTool generates Mermaid JS architecture flowcharts based on session history.
type ArchitecturalDiagrammerTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

func (t *ArchitecturalDiagrammerTool) Name() string {
	return "architectural_diagrammer"
}

func (t *ArchitecturalDiagrammerTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: REPORTING] [PHASE: TERMINAL] ARCHITECTURE TELEMETRY PROVIDER: Extracts structural project metrics and CSSA trace history into a dense JSON payload for downstream Mermaid diagram generation. [Routing Tags: diagram, architecture, mermaid, chart, telemetry-json]",
	}, t.Handle)
}

type DiagrammerInput struct {
	models.UniversalPipelineInput
}

func (t *ArchitecturalDiagrammerTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DiagrammerInput) (*mcp.CallToolResult, any, error) {
	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to load session: %v", err))
		return res, nil, nil
	}

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	// Generate the telemetry string securely.
	instructions := ""
	if instr, ok := input.Flags["instructions"].(string); ok {
		instructions = instr
	}

	var telemetryPayload string

	if recallAvailable && input.SessionID != "" {
		// Orchestrator path: extract deep telemetry from recall.
		telemetryString, err := t.Engine.ExtractArchitectureTelemetry(ctx, input.SessionID, input.Target, instructions)
		if err != nil {
			slog.Warn("[DIAGRAMMER] recall telemetry extraction failed, falling back to standalone", "error", err)
		} else {
			telemetryPayload = telemetryString.Data.TraceData

			// Enrich with go-refactor traceMap.
			if session.ProjectRoot != "" {
				if tm, tmErr := t.Engine.ExternalClient.AggregateSessionFromRecall(ctx, "go-refactor", session.ProjectRoot); tmErr == nil && tm != nil {
					if tmJSON, jErr := json.Marshal(tm); jErr == nil {
						telemetryPayload = telemetryPayload + "\n--- go-refactor AST trace ---\n" + string(tmJSON)
					}
				}
			}
		}
	}

	// Standalone fallback: return local session metadata as simplified JSON.
	if telemetryPayload == "" {
		localData := map[string]any{
			"mode":         "standalone",
			"project_root": session.ProjectRoot,
			"project_name": session.ProjectName,
			"language":     session.Language,
			"status":       session.Status,
			"gap_count":    len(session.Gaps),
			"instructions": instructions,
		}
		if session.Metadata != nil {
			localData["metadata_keys"] = metadataKeys(session.Metadata)
		}
		payload, _ := json.MarshalIndent(localData, "", "  ")
		telemetryPayload = string(payload)
	}

	// Store a note that we visualized it.
	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["last_diagram_type"] = input.Context

	if session.Artifacts == nil {
		session.Artifacts = make(map[string]string)
	}
	session.Artifacts["blueprint_telemetry"] = telemetryPayload

	if err := t.Manager.SaveSession(ctx, session); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("save session failed: %v", err))
		return res, nil, nil
	}

	if recallAvailable && session.ProjectRoot != "" {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, session.ProjectRoot, "telemetry_extracted", "native", "architectural_diagrammer", "", session.Metadata)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: "```json:data\n" + telemetryPayload + "\n```",
			},
		},
	}, nil, nil
}

// metadataKeys returns the top-level keys from session metadata for lightweight standalone telemetry.
func metadataKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
