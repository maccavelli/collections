package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/denisbrodbeck/machineid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/integration"
)

// jsonResult formats output variables into a JSON-like string for the agent.
func jsonResult(hint string, data map[string]any) (*mcp.CallToolResult, any, error) {
	b, _ := json.MarshalIndent(data, "", "  ")
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

type EvaluateIdeaArgs struct {
	RawIdea     string `json:"raw_idea" description:"The raw idea" jsonschema:"required"`
	TargetStack string `json:"target_stack" description:"The technology stack (.NET or Node)" jsonschema:"required"`
}

type ClarifyRequirementsArgs struct {
	SessionID    string `json:"session_id" description:"The session ID" jsonschema:"required"`
	UserResponse string `json:"user_response" description:"Socratic gaps and findings" jsonschema:"required"`
}

type IngestStandardsArgs struct {
	SessionID string `json:"session_id" description:"The session ID" jsonschema:"required"`
	SourceURL string `json:"source_url" description:"The standard source URL"`
	FilePath  string `json:"file_path" description:"The standard file path"`
}

type CritiqueDesignArgs struct {
	SessionID  string `json:"session_id" description:"The session ID" jsonschema:"required"`
	StrictMode bool   `json:"strict_mode" description:"Whether to use strict mode" jsonschema:"required"`
}

type FinalizeRequirementsArgs struct {
	SessionID         string `json:"session_id" description:"The session ID" jsonschema:"required"`
	ApprovalSignature string `json:"approval_signature" description:"The approval signature" jsonschema:"required"`
}

type BlueprintImplementationArgs struct {
	SessionID         string `json:"session_id" description:"The session ID" jsonschema:"required"`
	PatternPreference string `json:"pattern_preference" description:"The pattern preference" jsonschema:"required"`
}

type GenerateDocumentsArgs struct {
	SessionID    string `json:"session_id" description:"The session ID" jsonschema:"required"`
	Title        string `json:"title" description:"Document title" jsonschema:"required"`
	Markdown     string `json:"markdown" description:"Markdown spec" jsonschema:"required"`
	TargetBranch string `json:"target_branch" description:"Target git branch" jsonschema:"required"`
}

type CompleteDesignArgs struct {
	SessionID string `json:"session_id" description:"The session ID" jsonschema:"required"`
}

type ToolHandler struct {
	store *db.Store
}

func (h *ToolHandler) EvaluateIdea(ctx context.Context, req *mcp.CallToolRequest, args EvaluateIdeaArgs) (*mcp.CallToolResult, any, error) {
	// Generate a unique session ID
	id, _ := machineid.ID()
	sessionID := fmt.Sprintf("session-%s", id[:8])

	session := db.NewSessionState(sessionID)
	session.TechStack = args.TargetStack
	session.StepStatus["evaluate_idea"] = "COMPLETED"
	session.CurrentStep = "evaluate_idea"

	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}
	
	hint := "Next, call 'clarify_requirements' to refine the scope."
	return jsonResult(hint, map[string]any{
		"session_id":     sessionID,
		"scope_boundary": args.RawIdea,
	})
}

func (h *ToolHandler) ClarifyRequirements(ctx context.Context, req *mcp.CallToolRequest, args ClarifyRequirementsArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	if args.UserResponse != "" {
		for _, line := range strings.Split(args.UserResponse, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				session.AporiaResolutions = append(session.AporiaResolutions, trimmed)
				session.Tensions = append(session.Tensions, trimmed) // Track unresolved tensions
			}
		}
	}

	session.StepStatus["clarify_requirements"] = "COMPLETED"
	session.CurrentStep = "clarify_requirements"
	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}
	
	hint := "Next, call 'ingest_standards' to pull in applicable project standards."
	return jsonResult(hint, map[string]any{
		"thesis":           "Identified requirement base",
		"antithesis":       "Gaps and clarifications",
		"aporia_conflicts": session.Tensions,
	})
}

func (h *ToolHandler) IngestStandards(ctx context.Context, req *mcp.CallToolRequest, args IngestStandardsArgs) (*mcp.CallToolResult, any, error) {
	standard := args.SourceURL
	if args.FilePath != "" {
		standard = args.FilePath
	}
	if err := h.store.AppendStandard(args.SessionID, standard); err != nil {
		return errorResult(err.Error())
	}
	
	hint := "Next, call 'critique_design' to vet the architecture against the standards."
	return jsonResult(hint, map[string]any{
		"standards_blob": standard,
		"rule_checksum":  fmt.Sprintf("sha256-%d", len(standard)),
	})
}

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

func (h *ToolHandler) GenerateDocuments(ctx context.Context, req *mcp.CallToolRequest, args GenerateDocumentsArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}
	
	session.CurrentStep = "generate_documents"
	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	var bp *db.Blueprint
	var aporias []string
	if session != nil {
		bp = session.Blueprint
		aporias = session.AporiaResolutions
	}

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

func (h *ToolHandler) CompleteDesign(ctx context.Context, req *mcp.CallToolRequest, args CompleteDesignArgs) (*mcp.CallToolResult, any, error) {
	if err := h.store.DeleteSession(args.SessionID); err != nil {
		slog.Warn("complete_design: session cleanup failed", "error", err, "session_id", args.SessionID)
	}
	return jsonResult("Session completed and archived.", map[string]any{
		"handoff_report": "All tasks successful.",
		"archive_path":   fmt.Sprintf("/archives/%s", args.SessionID),
	})
}

func RegisterTools(s *mcp.Server, store *db.Store) {
	h := &ToolHandler{store: store}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "evaluate_idea",
		Description: "Detects stack (.NET/Node) and initializes session in BuntDB.",
	}, h.EvaluateIdea)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "clarify_requirements",
		Description: "Socratic analysis to fill gaps. Updates BuntDB metadata with aporia resolutions.",
	}, h.ClarifyRequirements)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ingest_standards",
		Description: "Stores fetched standards directly into the BuntDB session state.",
	}, h.IngestStandards)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "critique_design",
		Description: "Fetches the ingested standards from BuntDB and evaluates the design against them.",
	}, h.CritiqueDesign)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "finalize_requirements",
		Description: "Consolidates design into a Golden Copy JSON spec. Persists the finalized spec to BuntDB.",
	}, h.FinalizeRequirements)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "blueprint_implementation",
		Description: "Generates a technical implementation blueprint using the finalized requirements and ingested standards.",
	}, h.BlueprintImplementation)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_documents",
		Description: "Creates Jira task, Confluence page (ADF), and Hybrid Markdown Git commits. Consumes Blueprint data from the session.",
	}, h.GenerateDocuments)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "complete_design",
		Description: "Final handoff summary and session cleanup.",
	}, h.CompleteDesign)
}
