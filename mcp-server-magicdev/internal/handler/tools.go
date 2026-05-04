package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/integration"
)

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
	SessionID string `json:"session_id" description:"The session ID" jsonschema:"required"`
	TechStack string `json:"tech_stack" description:"The technology stack (.NET or Node)" jsonschema:"required"`
}

type ClarifyRequirementsArgs struct {
	SessionID string `json:"session_id" description:"The session ID" jsonschema:"required"`
	Findings  string `json:"findings" description:"Socratic gaps and findings" jsonschema:"required"`
}

type IngestStandardArgs struct {
	SessionID string `json:"session_id" description:"The session ID" jsonschema:"required"`
	Standard  string `json:"standard" description:"The standard to ingest" jsonschema:"required"`
}

type CritiqueDesignArgs struct {
	SessionID string `json:"session_id" description:"The session ID" jsonschema:"required"`
	Design    string `json:"design" description:"The design to critique" jsonschema:"required"`
}

type FinalizeRequirementsArgs struct {
	SessionID  string `json:"session_id" description:"The session ID" jsonschema:"required"`
	GoldenSpec string `json:"golden_spec" description:"The finalized golden spec" jsonschema:"required"`
}

type BlueprintImplementationArgs struct {
	SessionID string `json:"session_id" description:"The session ID" jsonschema:"required"`
}

type GenerateDocumentsArgs struct {
	SessionID string `json:"session_id" description:"The session ID" jsonschema:"required"`
	Title     string `json:"title" description:"Document title" jsonschema:"required"`
	Markdown  string `json:"markdown" description:"Markdown spec" jsonschema:"required"`
	RepoPath  string `json:"repo_path" description:"Repository path" jsonschema:"required"`
}

type CompleteDesignArgs struct {
	SessionID string `json:"session_id" description:"The session ID" jsonschema:"required"`
}

type ToolHandler struct {
	store *db.Store
}

func (h *ToolHandler) EvaluateIdea(ctx context.Context, req *mcp.CallToolRequest, args EvaluateIdeaArgs) (*mcp.CallToolResult, any, error) {
	session := db.NewSessionState(args.SessionID)
	session.TechStack = args.TechStack
	session.StepStatus["evaluate_idea"] = "COMPLETED"
	session.CurrentStep = "evaluate_idea"

	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}
	return textResult(fmt.Sprintf("Session %s initialized for %s stack.", args.SessionID, args.TechStack))
}

func (h *ToolHandler) ClarifyRequirements(ctx context.Context, req *mcp.CallToolRequest, args ClarifyRequirementsArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	if args.Findings != "" {
		for _, line := range strings.Split(args.Findings, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				session.AporiaResolutions = append(session.AporiaResolutions, trimmed)
			}
		}
	}

	session.StepStatus["clarify_requirements"] = "COMPLETED"
	session.CurrentStep = "clarify_requirements"
	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}
	return textResult("Requirements clarified and aporia resolutions saved.")
}

func (h *ToolHandler) IngestStandard(ctx context.Context, req *mcp.CallToolRequest, args IngestStandardArgs) (*mcp.CallToolResult, any, error) {
	if err := h.store.AppendStandard(args.SessionID, args.Standard); err != nil {
		return errorResult(err.Error())
	}
	return textResult("Standard successfully ingested.")
}

func (h *ToolHandler) CritiqueDesign(ctx context.Context, req *mcp.CallToolRequest, args CritiqueDesignArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}
	standardsJSON, err := json.Marshal(session.Standards)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to marshal standards: %v", err))
	}
	return textResult(fmt.Sprintf("Evaluate the following design:\n\n%s\n\nAgainst these standards:\n%s", args.Design, string(standardsJSON)))
}

func (h *ToolHandler) FinalizeRequirements(ctx context.Context, req *mcp.CallToolRequest, args FinalizeRequirementsArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	session.FinalSpec = args.GoldenSpec
	session.StepStatus["finalize_requirements"] = "COMPLETED"
	session.CurrentStep = "finalize_requirements"
	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}
	return textResult("Golden copy finalized and persisted.")
}

func (h *ToolHandler) BlueprintImplementation(ctx context.Context, req *mcp.CallToolRequest, args BlueprintImplementationArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	if session.StepStatus["finalize_requirements"] != "COMPLETED" {
		return errorResult("finalize_requirements must be completed before blueprint_implementation")
	}

	if session.FinalSpec == "" {
		return errorResult("no finalized spec found in session; finalize_requirements may not have persisted the golden copy")
	}

	slog.Info("blueprint_implementation: generating technical blueprint",
		"session_id", args.SessionID,
		"tech_stack", session.TechStack,
		"standards_count", len(session.Standards),
		"aporias_count", len(session.AporiaResolutions),
	)

	samplingPrompt := buildBlueprintSamplingPrompt(session)

	blueprint, samplingErr := attemptSampling(ctx, req, samplingPrompt, session.TechStack)
	if samplingErr != nil {
		slog.Warn("blueprint_implementation: sampling unavailable, returning structured prompt",
			"error", samplingErr,
		)

		session.StepStatus["blueprint_implementation"] = "AWAITING_SAMPLING"
		session.CurrentStep = "blueprint_implementation"
		if err := h.store.SaveSession(session); err != nil {
			slog.Error("blueprint_implementation: failed to save fallback state", "error", err)
		}

		return textResult(fmt.Sprintf(
			"Sampling unavailable. Please generate the blueprint JSON by analyzing the following context and responding with valid JSON matching the Blueprint schema.\n\n%s\n\n**Expected JSON Schema:**\n```json\n{\n  \"implementation_strategy\": {\"<requirement>\": \"<pattern>\"},\n  \"dependency_manifest\": [{\"name\": \"<pkg>\", \"version\": \"<ver>\", \"ecosystem\": \"nuget|npm\"}],\n  \"complexity_scores\": {\"<feature>\": <1-13>},\n  \"aporia_traceability\": {\"<contradiction>\": \"<resolution pattern>\"}\n}\n```",
			samplingPrompt,
		))
	}

	if err := h.store.SaveBlueprint(args.SessionID, blueprint); err != nil {
		return errorResult(fmt.Sprintf("failed to save blueprint: %v", err))
	}
	if err := h.store.UpdateCurrentStep(args.SessionID, "blueprint_implementation"); err != nil {
		slog.Error("blueprint_implementation: failed to update step", "error", err)
	}

	_ = h.store.AppendStepStatus(args.SessionID, "blueprint_implementation", "COMPLETED")

	summary := buildBlueprintSummary(blueprint, session.TechStack)

	slog.Info("blueprint_implementation: completed",
		"session_id", args.SessionID,
		"strategies", len(blueprint.ImplementationStrategy),
		"dependencies", len(blueprint.DependencyManifest),
		"features_scored", len(blueprint.ComplexityScores),
	)

	return textResult(summary)
}

func (h *ToolHandler) GenerateDocuments(ctx context.Context, req *mcp.CallToolRequest, args GenerateDocumentsArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		slog.Warn("generate_documents: could not load session, proceeding without blueprint", "error", err)
	}

	var bp *db.Blueprint
	var aporias []string
	if session != nil {
		bp = session.Blueprint
		aporias = session.AporiaResolutions
	}

	if err := integration.ProcessDocumentGeneration(args.Title, args.Markdown, args.RepoPath, args.SessionID, bp, aporias); err != nil {
		return errorResult(err.Error())
	}
	return textResult("Documents successfully generated and pushed.")
}

func (h *ToolHandler) CompleteDesign(ctx context.Context, req *mcp.CallToolRequest, args CompleteDesignArgs) (*mcp.CallToolResult, any, error) {
	if err := h.store.DeleteSession(args.SessionID); err != nil {
		slog.Warn("complete_design: session cleanup failed", "error", err, "session_id", args.SessionID)
	}
	return textResult("Session completed and archived.")
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
		Name:        "ingest_standard",
		Description: "Stores fetched standards directly into the BuntDB session state.",
	}, h.IngestStandard)

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
		Description: "Generates a technical implementation blueprint using the finalized requirements and ingested standards. Maps requirements to .NET 9+/Node 24+ patterns, produces a dependency manifest, and estimates complexity scores for Jira story points.",
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

func buildBlueprintSamplingPrompt(session *db.SessionState) string {
	var b strings.Builder

	b.WriteString("## Technical Blueprint Generation\n\n")
	b.WriteString(fmt.Sprintf("**Target Stack:** %s\n\n", session.TechStack))

	b.WriteString("### Finalized Requirements (Golden Copy)\n")
	b.WriteString(session.FinalSpec)
	b.WriteString("\n\n")

	if len(session.Standards) > 0 {
		b.WriteString("### Ingested Project Standards\n")
		for i, std := range session.Standards {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, std))
		}
		b.WriteString("\n")
	}

	if len(session.AporiaResolutions) > 0 {
		b.WriteString("### Aporia Resolutions (Contradictions Found)\n")
		for i, aporia := range session.AporiaResolutions {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, aporia))
		}
		b.WriteString("\n")
	}

	b.WriteString("### Instructions\n")
	b.WriteString("Based on the finalized requirements, standards, and aporia resolutions above, generate a JSON Blueprint with:\n")
	b.WriteString("1. `implementation_strategy`: Map each requirement to the appropriate ")

	switch strings.ToLower(session.TechStack) {
	case ".net", "dotnet":
		b.WriteString(".NET 9+ pattern (Minimal APIs, C# 13 features, DI patterns).\n")
	case "node", "nodejs":
		b.WriteString("Node 24+ pattern (TypeScript 5.x, ESM, Clean Architecture folders).\n")
	default:
		b.WriteString("appropriate tech pattern.\n")
	}

	b.WriteString("2. `dependency_manifest`: List required NuGet/NPM packages with versions.\n")
	b.WriteString("3. `complexity_scores`: Estimate 1-13 story points per feature.\n")
	b.WriteString("4. `aporia_traceability`: For each contradiction, describe how the code resolves it.\n")

	return b.String()
}

func attemptSampling(ctx context.Context, req *mcp.CallToolRequest, prompt, techStack string) (*db.Blueprint, error) {
	if req == nil || req.Session == nil {
		return nil, fmt.Errorf("no client session available for sampling")
	}

	result, err := req.Session.CreateMessage(ctx, &mcp.CreateMessageParams{
		Messages: []*mcp.SamplingMessage{
			{
				Role: "user",
				Content: &mcp.TextContent{
					Text: prompt + "\n\nRespond ONLY with valid JSON matching the Blueprint schema. No markdown fences.",
				},
			},
		},
		MaxTokens: 4096,
		SystemPrompt: fmt.Sprintf(
			"You are a technical architect generating implementation blueprints for %s projects. Output only valid JSON.",
			techStack,
		),
	})
	if err != nil {
		return nil, fmt.Errorf("sampling request failed: %w", err)
	}

	textContent, ok := result.Content.(*mcp.TextContent)
	if !ok || textContent.Text == "" {
		return nil, fmt.Errorf("sampling returned non-text or empty content")
	}

	var bp db.Blueprint
	responseText := strings.TrimSpace(textContent.Text)

	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	if err := json.Unmarshal([]byte(responseText), &bp); err != nil {
		return nil, fmt.Errorf("failed to parse sampling response as Blueprint JSON: %w", err)
	}

	return &bp, nil
}

func buildBlueprintSummary(bp *db.Blueprint, techStack string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Blueprint Generated (%s)\n\n", techStack))

	if len(bp.ImplementationStrategy) > 0 {
		b.WriteString("### Implementation Strategy\n")
		for req, pattern := range bp.ImplementationStrategy {
			b.WriteString(fmt.Sprintf("- **%s** → %s\n", req, pattern))
		}
		b.WriteString("\n")
	}

	if len(bp.DependencyManifest) > 0 {
		b.WriteString("### Dependency Manifest\n")
		for _, dep := range bp.DependencyManifest {
			b.WriteString(fmt.Sprintf("- %s@%s (%s)\n", dep.Name, dep.Version, dep.Ecosystem))
		}
		b.WriteString("\n")
	}

	if len(bp.ComplexityScores) > 0 {
		b.WriteString("### Complexity Scores (Story Points)\n")
		totalPoints := 0
		for feature, points := range bp.ComplexityScores {
			b.WriteString(fmt.Sprintf("- %s: %d SP\n", feature, points))
			totalPoints += points
		}
		b.WriteString(fmt.Sprintf("\n**Total Estimated Points:** %d\n\n", totalPoints))
	}

	if len(bp.AporiaTraceability) > 0 {
		b.WriteString("### Aporia Traceability\n")
		for contradiction, resolution := range bp.AporiaTraceability {
			b.WriteString(fmt.Sprintf("- **%s** → %s\n", contradiction, resolution))
		}
	}

	return b.String()
}
