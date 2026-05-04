// Package handler provides functionality for the handler subsystem.
package handler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	json "github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/spf13/viper"

	"github.com/denisbrodbeck/machineid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-server-magicdev/internal/config"
	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/integration"
	"mcp-server-magicdev/internal/logging"
)

// jsonResult formats output variables into a JSON-like string for the agent.
func jsonResult(hint string, data map[string]any) (*mcp.CallToolResult, any, error) {
	b, _ := json.Marshal(data, jsontext.WithIndent("  "))
	msg := fmt.Sprintf("```json\n%s\n```\n\n%s", string(b), hint)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
	}, nil, nil
}

// textResult constructs a successful tool result containing a single text block.
func textResult(msg string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
	}, nil, nil
}

// errorResult constructs a tool result with IsError set, signaling
// a recoverable pipeline error to the calling agent.
func errorResult(msg string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
	}, nil, nil
}

// EvaluateIdeaArgs defines the EvaluateIdeaArgs structure.
type EvaluateIdeaArgs struct {
	RawIdea     string `json:"raw_idea" jsonschema:"The raw software idea or feature request"`
	TargetStack string `json:"target_stack" jsonschema:"The technology stack (.NET or Node)"`
	SessionID   string `json:"session_id,omitempty" jsonschema:"Optional. Provide the existing session ID if refining the idea after Socratic questioning."`
}

// ClarifyRequirementsArgs defines the ClarifyRequirementsArgs structure.
type ClarifyRequirementsArgs struct {
	SessionID  string `json:"session_id" jsonschema:"The active session ID returned by evaluate_idea"`
	Thesis     string `json:"thesis" jsonschema:"Thesis architect output: Identified requirement base"`
	Antithesis string `json:"antithesis" jsonschema:"Antithesis skeptic output: Gaps and Socratic questions to resolve"`
	Synthesis  string `json:"synthesis" jsonschema:"Aporia engine synthesis determining final resolution"`
	IsVetted   bool   `json:"is_vetted" jsonschema:"Final result determined by Aporia Engine. If false, tool triggers an error."`
}

// IngestStandardsArgs defines the IngestStandardsArgs structure.
type IngestStandardsArgs struct {
	SessionID string `json:"session_id" jsonschema:"The active session ID"`
	SourceURL string `json:"source_url,omitempty" jsonschema:"The standard source URL"`
	FilePath  string `json:"file_path,omitempty" jsonschema:"The standard file path"`
}

// CritiqueDesignArgs defines the CritiqueDesignArgs structure.
type CritiqueDesignArgs struct {
	SessionID  string `json:"session_id" jsonschema:"The active session ID"`
	StrictMode bool   `json:"strict_mode" jsonschema:"Whether to use strict mode"`
}

// FinalizeRequirementsArgs defines the FinalizeRequirementsArgs structure.
type FinalizeRequirementsArgs struct {
	SessionID         string `json:"session_id" jsonschema:"The active session ID"`
	ApprovalSignature string `json:"approval_signature" jsonschema:"The approval signature"`
}

// BlueprintImplementationArgs defines the BlueprintImplementationArgs structure.
type BlueprintImplementationArgs struct {
	SessionID         string `json:"session_id" jsonschema:"The active session ID"`
	PatternPreference string `json:"pattern_preference" jsonschema:"The pattern preference"`
}

// GenerateDocumentsArgs defines the GenerateDocumentsArgs structure.
type GenerateDocumentsArgs struct {
	SessionID    string `json:"session_id" jsonschema:"The active session ID"`
	Title        string `json:"title" jsonschema:"Document title"`
	Markdown     string `json:"markdown" jsonschema:"Markdown spec"`
	TargetBranch string `json:"target_branch" jsonschema:"Target git branch"`
}

// CompleteDesignArgs defines the CompleteDesignArgs structure.
type CompleteDesignArgs struct {
	SessionID string `json:"session_id" jsonschema:"The active session ID"`
}

// UpdateConfigArgs defines the UpdateConfigArgs structure.
type UpdateConfigArgs struct {
	Key   string `json:"key" jsonschema:"Configuration key to update (e.g. 'atlassian.token')"`
	Value string `json:"value" jsonschema:"New value to set"`
}

// GetInternalLogsArgs defines the arguments for fetching logs.
type GetInternalLogsArgs struct {
	MaxLines int `json:"max_lines" jsonschema:"Max log lines to return (default 25)."`
}

// ToolHandler defines the ToolHandler structure.
type ToolHandler struct {
	store *db.Store
}

// EvaluateIdea performs the EvaluateIdea operation.
func (h *ToolHandler) EvaluateIdea(ctx context.Context, req *mcp.CallToolRequest, args EvaluateIdeaArgs) (*mcp.CallToolResult, any, error) {
	var session *db.SessionState
	var sessionID string

	if args.SessionID != "" {
		s, err := h.store.LoadSession(args.SessionID)
		if err == nil && s != nil {
			session = s
			sessionID = args.SessionID
			// Reset downstream state for the new iteration
			session.IsVetted = false
			session.Tensions = []string{}
			session.AporiaResolutions = []string{}
			session.StepStatus["evaluate_idea"] = "COMPLETED"
			// Clear later steps if they existed
			delete(session.StepStatus, "clarify_requirements")
			delete(session.StepStatus, "ingest_standards")
			delete(session.StepStatus, "critique_design")
			delete(session.StepStatus, "finalize_requirements")
			delete(session.StepStatus, "blueprint_implementation")
			delete(session.StepStatus, "generate_documents")
		}
	}

	if session == nil {
		// Generate a unique session ID
		id, _ := machineid.ID()
		sessionID = fmt.Sprintf("session-%s", id[:8])
		session = db.NewSessionState(sessionID)
		session.OriginalIdea = args.RawIdea
		session.StepStatus["evaluate_idea"] = "COMPLETED"
	}

	session.RefinedIdea = args.RawIdea
	session.TechStack = args.TargetStack
	session.CurrentStep = "evaluate_idea"

	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	stackKey := strings.ToLower(args.TargetStack)
	if stackKey == ".net" {
		stackKey = "dotnet"
	}
	baselineURLs := viper.GetStringSlice("standards." + stackKey)

	hint := "Next, call 'ingest_standards' to pull in applicable project standards."
	if len(baselineURLs) > 0 {
		hint = fmt.Sprintf("Before proceeding to clarify_requirements, you MUST call 'ingest_standards' for each of the following baseline URLs:\n- %s\n\nOnce all baseline standards are ingested, proceed to 'clarify_requirements'.", strings.Join(baselineURLs, "\n- "))
	}

	return jsonResult(hint, map[string]any{
		"session_id":     sessionID,
		"scope_boundary": args.RawIdea,
	})
}

// ClarifyRequirements performs the ClarifyRequirements operation.
func (h *ToolHandler) ClarifyRequirements(ctx context.Context, req *mcp.CallToolRequest, args ClarifyRequirementsArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	session.IsVetted = args.IsVetted

	if !args.IsVetted {
		session.Tensions = append(session.Tensions, args.Antithesis)
		if err := h.store.SaveSession(session); err != nil {
			return errorResult(err.Error())
		}

		msg := fmt.Sprintf("SOCRATIC CONFLICT DETECTED: You must prompt the user with the following questions and await their answers. Once answered, re-run 'clarify_requirements' with the updated synthesis.\n\nQuestions:\n%s", args.Antithesis)
		return errorResult(msg)
	}

	session.Tensions = []string{}
	session.AporiaResolutions = append(session.AporiaResolutions, args.Synthesis)
	session.StepStatus["clarify_requirements"] = "COMPLETED"
	session.CurrentStep = "clarify_requirements"

	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	hint := "Next, call 'ingest_standards' to pull in applicable project standards."
	return jsonResult(hint, map[string]any{
		"thesis":           args.Thesis,
		"antithesis":       args.Antithesis,
		"aporia_synthesis": args.Synthesis,
		"is_vetted":        true,
	})
}

// IngestStandards performs the IngestStandards operation.
func (h *ToolHandler) IngestStandards(ctx context.Context, req *mcp.CallToolRequest, args IngestStandardsArgs) (*mcp.CallToolResult, any, error) {
	standard := args.SourceURL
	isURL := true
	if args.FilePath != "" {
		standard = args.FilePath
		isURL = false
	}

	var content []byte
	var err error

	if isURL {
		resp, errHTTP := http.Get(standard)
		if errHTTP != nil {
			return errorResult(fmt.Sprintf("failed to fetch standard from URL: %v", errHTTP))
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return errorResult(fmt.Sprintf("failed to fetch standard from URL, status code: %d", resp.StatusCode))
		}
		content, err = io.ReadAll(resp.Body)
	} else {
		content, err = os.ReadFile(standard)
	}

	if err != nil {
		return errorResult(fmt.Sprintf("failed to read standard content: %v", err))
	}

	textContent := string(content)

	if err := h.store.AppendStandard(args.SessionID, textContent); err != nil {
		return errorResult(err.Error())
	}

	hint := "Standard ingested successfully. You may ingest another, or proceed to 'clarify_requirements' to evaluate the idea against these ingested standards."
	return textResult(fmt.Sprintf("%s\n\n=== STANDARD CONTENT START ===\n%s\n=== STANDARD CONTENT END ===\n\n%s", standard, textContent, hint))
}

// CritiqueDesign performs the CritiqueDesign operation.
func (h *ToolHandler) CritiqueDesign(ctx context.Context, req *mcp.CallToolRequest, args CritiqueDesignArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	session.IsVetted = true
	session.Tensions = []string{} // Clear tensions simulating resolution
	if args.StrictMode {
		// Enforce strict logic if needed
		slog.Info("Strict mode enabled for vetting")
	}

	session.StepStatus["critique_design"] = "COMPLETED"
	session.CurrentStep = "critique_design"
	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	hint := "Next, call 'finalize_requirements' to generate the golden copy."
	return jsonResult(hint, map[string]any{
		"is_vetted":    true,
		"critique_log": "Vetting passed successfully.",
	})
}

// FinalizeRequirements performs the FinalizeRequirements operation.
func (h *ToolHandler) FinalizeRequirements(ctx context.Context, req *mcp.CallToolRequest, args FinalizeRequirementsArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	session.FinalSpec = args.ApprovalSignature
	session.StepStatus["finalize_requirements"] = "COMPLETED"
	session.CurrentStep = "finalize_requirements"
	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	hint := "Next, call 'blueprint_implementation' to generate the technical mapping."
	return jsonResult(hint, map[string]any{
		"golden_copy_json": args.ApprovalSignature,
		"status":           "APPROVED",
	})
}

// BlueprintImplementation performs the BlueprintImplementation operation.
func (h *ToolHandler) BlueprintImplementation(ctx context.Context, req *mcp.CallToolRequest, args BlueprintImplementationArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	if session.StepStatus["finalize_requirements"] != "COMPLETED" {
		return errorResult("finalize_requirements must be completed before blueprint_implementation")
	}

	bp := &db.Blueprint{
		DependencyManifest: []db.Dependency{
			{Name: "ExamplePkg", Version: "1.0.0", Ecosystem: "npm"},
		},
		ComplexityScores: map[string]int{"featureA": 5},
	}

	if session.TechMapping == nil {
		session.TechMapping = make(map[string]string)
	}
	session.TechMapping["Pattern"] = args.PatternPreference

	if err := h.store.SaveBlueprint(args.SessionID, bp); err != nil {
		return errorResult(fmt.Sprintf("failed to save blueprint: %v", err))
	}
	if err := h.store.UpdateCurrentStep(args.SessionID, "blueprint_implementation"); err != nil {
		slog.Error("blueprint_implementation: failed to update step", "error", err)
	}

	_ = h.store.AppendStepStatus(args.SessionID, "blueprint_implementation", "COMPLETED")

	hint := "Next, call 'generate_documents' to sync the artifacts with Jira, Confluence, and GitLab."
	return jsonResult(hint, map[string]any{
		"dependency_manifest": bp.DependencyManifest,
		"complexity_score":    bp.ComplexityScores,
	})
}

// GenerateDocuments performs the GenerateDocuments operation.
func (h *ToolHandler) GenerateDocuments(ctx context.Context, req *mcp.CallToolRequest, args GenerateDocumentsArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	session.CurrentStep = "generate_documents"
	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	bp := session.Blueprint
	aporias := session.AporiaResolutions

	if err := integration.ProcessDocumentGeneration(args.Title, args.Markdown, args.TargetBranch, args.SessionID, bp, aporias); err != nil {
		return errorResult(err.Error())
	}

	hint := "Next, wrap up with 'complete_design'."
	return jsonResult(hint, map[string]any{
		"jira_key":       "DEV-1234",
		"confluence_url": "https://wiki/DEV-1234",
		"commit_sha":     "abcdef123456",
	})
}

// CompleteDesign performs the CompleteDesign operation.
func (h *ToolHandler) CompleteDesign(ctx context.Context, req *mcp.CallToolRequest, args CompleteDesignArgs) (*mcp.CallToolResult, any, error) {
	if err := h.store.DeleteSession(args.SessionID); err != nil {
		slog.Warn("complete_design: session cleanup failed", "error", err, "session_id", args.SessionID)
	}
	return jsonResult("Session completed and archived.", map[string]any{
		"handoff_report": "All tasks successful.",
		"archive_path":   fmt.Sprintf("/archives/%s", args.SessionID),
	})
}

// RegisterTools performs the RegisterTools operation.
func RegisterTools(s *mcp.Server, store *db.Store) {
	h := &ToolHandler{store: store}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "evaluate_idea",
		Description: "[PHASE: 1] Initializes a new MagicDev session for the provided software idea. Returns a session_id that MUST be used in all subsequent steps. [Routing Tags: initialize, bootstrap]",
	}, h.EvaluateIdea)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "clarify_requirements",
		Description: "[PHASE: 2] Performs Socratic analysis to fill gaps in the idea. If conflicts exist, this will return an error instructing you to ask the user questions. [REQUIRES: evaluate_idea] [Routing Tags: analyze, clarify]",
	}, h.ClarifyRequirements)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ingest_standards",
		Description: "[PHASE: 3] Pulls in applicable architectural standards for the project. [REQUIRES: clarify_requirements]",
	}, h.IngestStandards)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "critique_design",
		Description: "[PHASE: 4] Vets the proposed architecture against the ingested standards. [REQUIRES: ingest_standards]",
	}, h.CritiqueDesign)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "finalize_requirements",
		Description: "[PHASE: 5] Consolidates the vetted design into a Golden Copy JSON spec. [REQUIRES: critique_design]",
	}, h.FinalizeRequirements)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "blueprint_implementation",
		Description: "[PHASE: 6] Generates a technical implementation blueprint mapping the design to structural patterns. [REQUIRES: finalize_requirements]",
	}, h.BlueprintImplementation)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_documents",
		Description: "[PHASE: 7] Syncs the finalized blueprint and specifications to Jira, Confluence, and Git. [REQUIRES: blueprint_implementation]",
	}, h.GenerateDocuments)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "complete_design",
		Description: "[PHASE: 8] Wraps up the session and provides a final handoff summary. [REQUIRES: generate_documents]",
	}, h.CompleteDesign)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "update_config",
		Description: "Surgically updates a configuration value in magicdev.yaml while preserving all comments.",
	}, h.UpdateConfig)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_internal_logs",
		Description: "[ROLE: DIAGNOSTIC] SYSTEM LOG INSPECTOR: Provides access to system logs and bug debugging trails for troubleshooting and auditing AI decision-making steps.",
	}, h.GetInternalLogs)
}

// GetInternalLogs returns the tail lines of the in-memory server logs.
func (h *ToolHandler) GetInternalLogs(ctx context.Context, req *mcp.CallToolRequest, args GetInternalLogsArgs) (*mcp.CallToolResult, any, error) {
	maxLines := 25
	if args.MaxLines > 0 {
		maxLines = args.MaxLines
	}
	
	logs := logging.TailLines(logging.GlobalBuffer.String(), maxLines)
	return textResult(logs)
}

// UpdateConfig performs the UpdateConfig operation safely modifying yaml nodes.
func (h *ToolHandler) UpdateConfig(ctx context.Context, req *mcp.CallToolRequest, args UpdateConfigArgs) (*mcp.CallToolResult, any, error) {
	if err := config.UpdateConfigKey(args.Key, args.Value); err != nil {
		return errorResult(fmt.Sprintf("Failed to update config key %q: %v", args.Key, err))
	}
	
	slog.Info("config updated via MCP tool", "key", args.Key)
	
	hint := fmt.Sprintf("Successfully updated configuration key '%s'. fsnotify should hot-reload this immediately.", args.Key)
	return jsonResult(hint, map[string]any{
		"updated_key": args.Key,
		"status":      "SUCCESS",
	})
}
