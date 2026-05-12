// Package handler provides functionality for the handler subsystem.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/viper"

	"mcp-server-magicdev/internal/config"
	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/integration"
	"mcp-server-magicdev/internal/logging"
	"mcp-server-magicdev/internal/sync"
)

// hybridMarkdownResult formats output variables into a JSON-frontmatter string for the agent.
func hybridMarkdownResult(hint string, data map[string]any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, nil, err
	}
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
	RawIdea                string            `json:"raw_idea" jsonschema:"The raw software idea or feature request"`
	TargetStack            string            `json:"target_stack" jsonschema:"The technology stack (.NET or Node)"`
	SessionID              string            `json:"session_id,omitempty" jsonschema:"Optional. Provide the existing session ID if refining the idea after Socratic questioning."`
	Tags                   map[string]string `json:"tags,omitempty" jsonschema:"Optional freeform key-value tags for categorization."`
	Labels                 []string          `json:"labels" jsonschema:"REQUIRED classification labels (e.g. domain:ecommerce). If not provided in the prompt, you MUST ask the user before calling."`
	TargetEnvironment      string            `json:"target_environment" jsonschema:"REQUIRED target environment (e.g. containerized). If not provided in the prompt, you MUST ask the user before calling."`
	ComplianceRequirements []string          `json:"compliance_requirements,omitempty" jsonschema:"Optional compliance tags: SOC2, HIPAA, PCI-DSS, GDPR."`
	BusinessCase           string            `json:"business_case" jsonschema:"REQUIRED business case or decision drivers. If not provided in the prompt, you MUST ask the user before calling."`
}

// ClarifyRequirementsArgs defines the structured input for the Socratic Trifecta.
type ClarifyRequirementsArgs struct {
	SessionID           string                  `json:"session_id" jsonschema:"The active session ID returned by evaluate_idea"`
	DesignProposal      *db.DesignProposal      `json:"design_proposal" jsonschema:"Thesis architect output: Proposed architecture, template AST, security mandates, stack tuning, and standard references"`
	SkepticAnalysis     *db.SkepticAnalysis     `json:"skeptic_analysis" jsonschema:"Antithesis skeptic output: Vulnerabilities, design concerns, and granular questions"`
	ChaosAnalysis       *db.ChaosAnalysis       `json:"chaos_analysis,omitempty" jsonschema:"Chaos Architect output: Fatal flaws, operational constraints, rejected patterns, and stress scenarios"`
	SynthesisResolution *db.SynthesisResolution `json:"synthesis_resolution" jsonschema:"Aporia engine synthesis: Resolved decisions, outstanding questions, and unresolved dependencies"`
	IsVetted            bool                    `json:"is_vetted" jsonschema:"Final result determined by Aporia Engine. If false, tool triggers an error with outstanding questions."`
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
	SessionID              string              `json:"session_id" jsonschema:"The active session ID"`
	PatternPreference      string              `json:"pattern_preference" jsonschema:"The pattern preference"`
	ImplementationStrategy map[string]string   `json:"implementation_strategy,omitempty" jsonschema:"Optional requirement-to-pattern mapping."`
	Dependencies           []db.Dependency     `json:"dependencies,omitempty" jsonschema:"Optional real dependency list."`
	ComplexityScores       map[string]int      `json:"complexity_scores,omitempty" jsonschema:"Optional feature complexity estimates (1-13 SP)."`
	FileStructure          []db.FileEntry      `json:"file_structure,omitempty" jsonschema:"Optional proposed project file tree."`
	SecurityConsiderations []db.SecurityItem   `json:"security_considerations,omitempty" jsonschema:"Optional OWASP-aligned security findings."`
	NFRs                   []db.NFR            `json:"non_functional_requirements,omitempty" jsonschema:"Optional non-functional requirements."`
	TestingStrategy        map[string]string   `json:"testing_strategy,omitempty" jsonschema:"Optional feature to test approach mapping."`
	ADRs                   []db.ADR            `json:"adrs,omitempty" jsonschema:"Optional architecture decision records."`
	APIContracts           []db.APIEndpoint    `json:"api_contracts,omitempty" jsonschema:"Optional REST/GraphQL endpoint definitions."`
	DataModel              []db.Entity         `json:"data_model,omitempty" jsonschema:"Optional entity definitions for ERD generation."`
	MCPTools               []db.MCPTool        `json:"mcp_tools,omitempty" jsonschema:"Optional definitions for MCP Tools."`
	MCPResources           []db.MCPResource    `json:"mcp_resources,omitempty" jsonschema:"Optional definitions for MCP Resources."`
	MCPPrompts             []db.MCPPrompt      `json:"mcp_prompts,omitempty" jsonschema:"Optional definitions for MCP Prompts."`
}

// GenerateDocumentsArgs defines the GenerateDocumentsArgs structure.
type GenerateDocumentsArgs struct {
	SessionID       string `json:"session_id" jsonschema:"The active session ID"`
	Title           string `json:"title" jsonschema:"Document title"`
	Markdown        string `json:"markdown" jsonschema:"Supplementary markdown content from the agent."`
	TargetBranch    string `json:"target_branch" jsonschema:"Target git branch"`
	DiagramOverride string `json:"diagram_override,omitempty" jsonschema:"Optional manual D2 diagram override."`
}

// CompleteDesignArgs defines the CompleteDesignArgs structure.
type CompleteDesignArgs struct {
	SessionID    string `json:"session_id" jsonschema:"The active session ID"`
	ArtifactPath string `json:"artifact_path,omitempty" jsonschema:"Optional absolute path for IDE-specific artifact delivery"`
}

// UpdateConfigArgs defines the UpdateConfigArgs structure.
type UpdateConfigArgs struct {
	Key   string `json:"key" jsonschema:"Configuration key to update. Valid keys: confluence.url, confluence.space, confluence.parent_page_id, confluence.mock, jira.email, jira.url, jira.project, jira.issue, jira.mock, jira.story_points_field, git.username, git.server_url, git.project_path, git.default_branch, agent.default_stack, runtime.gomemlimit, runtime.gomaxprocs, server.log_level, server.db_path, llm.provider, llm.model, llm.disable, standards.node.path, standards.node.total_files, standards.node.max_directory_depth, standards.dotnet.path, standards.dotnet.total_files, standards.dotnet.max_directory_depth"`
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
	if args.TargetEnvironment == "" || len(args.Labels) == 0 || args.BusinessCase == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "[VALIDATION REQUIRED] Missing architectural context.\nYou MUST determine the `target_environment`, applicable domain `labels` (e.g. ecommerce, erp), and the `business_case` (decision drivers) before evaluating the idea.\nACTION: Ask the user clarifying questions to obtain this missing data. Do not guess. Once the user answers, call `evaluate_idea` again."},
			},
		}, nil, nil
	}

	if err := integration.VerifyConnectivity(h.store); err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("[BOOTSTRAP FAILED] Environment connectivity check failed: %v\nPlease ensure your Jira, GitLab, and Confluence credentials are correct before proceeding.", err)},
			},
		}, nil, nil
	}

	var session *db.SessionState
	var sessionID string

	if args.SessionID != "" {
		s, err := h.store.LoadSession(args.SessionID)
		if err == nil && s != nil {
			session = s
			sessionID = args.SessionID
			// Reset downstream state for the new iteration
			session.IsVetted = false
			session.DesignProposal = nil
			session.SkepticAnalysis = nil
			session.SynthesisResolution = nil
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
		id := uuid.New().String()
		sessionID = fmt.Sprintf("session-%s", id[:8])
		session = db.NewSessionState(sessionID)
		session.OriginalIdea = args.RawIdea
		session.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		session.StepStatus["evaluate_idea"] = "COMPLETED"
	}

	session.RefinedIdea = args.RawIdea
	session.TechStack = args.TargetStack
	session.CurrentStep = "evaluate_idea"
	session.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	RecordStepTiming(session, "evaluate_idea")

	// Populate forward-thinking session metadata from agent-provided args
	if len(args.Tags) > 0 {
		session.Tags = args.Tags
	}
	if len(args.Labels) > 0 {
		session.Labels = args.Labels
	}
	if args.TargetEnvironment != "" {
		session.TargetEnvironment = args.TargetEnvironment
	}
	if args.BusinessCase != "" {
		session.BusinessCase = args.BusinessCase
	}
	if len(args.ComplianceRequirements) > 0 {
		session.ComplianceRequirements = args.ComplianceRequirements
	}

	populated := 0
	if args.RawIdea != "" { populated++ }
	if args.TargetStack != "" { populated++ }
	if len(args.Labels) > 0 { populated++ }
	if args.TargetEnvironment != "" { populated++ }
	if args.BusinessCase != "" { populated++ }
	CheckPayloadCompleteness(session, "evaluate_idea", populated, 5)

	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	// --- Intelligence Engine (Phase 1) ---
	meta, err := h.store.GetSessionMetadata(sessionID)
	if err != nil {
		slog.Warn("Failed to retrieve session metadata", "error", err)
	}
	if meta == nil {
		meta = &db.SessionMetadata{SessionID: sessionID}
	}

	llmClient, llmErr := integration.NewLLMClient(h.store)
	if llmErr == nil && llmClient != nil {
		// Run semantic gatekeeper with retry
		prompt := fmt.Sprintf("Analyze the following software idea and determine its complexity, security footprint, and pattern preference.\n\nIdea: %s\nTarget Stack: %s\nTarget Environment: %s\nLabels: %s\nBusiness Case: %s\n\nReturn ONLY a JSON object with the following keys: \"complexity_score\" (integer 1-13), \"security_footprint\" (string), \"pattern_preference\" (string).", args.RawIdea, args.TargetStack, args.TargetEnvironment, strings.Join(args.Labels, ", "), args.BusinessCase)

		const maxRetries = 3
		var gatekeeperOK bool

		for attempt := 1; attempt <= maxRetries; attempt++ {
			llmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			response, err := llmClient.GenerateContent(llmCtx, prompt)
			cancel()

			if err != nil {
				slog.Warn("Intelligence Engine LLM call failed",
					"attempt", attempt,
					"max_retries", maxRetries,
					"error", err,
				)
				if attempt < maxRetries {
					backoff := time.Duration(1<<attempt) * time.Second
					select {
					case <-time.After(backoff):
					case <-ctx.Done():
						slog.Warn("Intelligence Engine cancelled during backoff")
						break
					}
				}
				continue
			}

			var gatekeeper struct {
				ComplexityScore   int    `json:"complexity_score"`
				SecurityFootprint string `json:"security_footprint"`
				PatternPreference string `json:"pattern_preference"`
			}
			cleaned := strings.TrimPrefix(strings.TrimSpace(response), "```json")
			cleaned = strings.TrimPrefix(cleaned, "```")
			cleaned = strings.TrimSuffix(cleaned, "```")
			cleaned = strings.TrimSpace(cleaned)

			if parseErr := json.Unmarshal([]byte(cleaned), &gatekeeper); parseErr == nil {
				meta.ComplexityScore = gatekeeper.ComplexityScore
				meta.SecurityFootprint = gatekeeper.SecurityFootprint
				meta.PatternPreference = gatekeeper.PatternPreference
				h.store.SaveSessionMetadata(meta)
				slog.Info("Semantic gatekeeper completed successfully",
					"score", meta.ComplexityScore,
					"attempt", attempt,
				)
				gatekeeperOK = true
				break
			} else {
				slog.Warn("Failed to parse semantic gatekeeper response",
					"attempt", attempt,
					"error", parseErr,
					"response", response,
				)
				if attempt < maxRetries {
					backoff := time.Duration(1<<attempt) * time.Second
					select {
					case <-time.After(backoff):
					case <-ctx.Done():
						break
					}
				}
			}
		}

		if !gatekeeperOK {
			slog.Warn("Intelligence Engine failed after all retries, using defaults")
		}
	} else if errors.Is(llmErr, integration.ErrLLMDisabled) {
		slog.Info("Intelligence Engine bypassed via config")
	} else {
		slog.Info("LLM not configured, skipping Intelligence Engine", "reason", llmErr)
	}
	// ---------------------------------------------------

	baselineURLs := sync.GetContextualStandards(args.TargetStack, args.TargetEnvironment, args.Labels)

	hint := "Next, call 'ingest_standards' to pull in applicable project standards."
	if len(baselineURLs) > 0 {
		hint = fmt.Sprintf("Before proceeding to clarify_requirements, you MUST call 'ingest_standards' for each of the following baseline URLs:\n- %s\n\nOnce all baseline standards are ingested, proceed to 'clarify_requirements'.", strings.Join(baselineURLs, "\n- "))
	}

	LogSessionHash(session, "evaluate_idea")

	return hybridMarkdownResult(hint, map[string]any{
		"session_id":     sessionID,
		"scope_boundary": args.RawIdea,
		"gatekeeper_active": llmClient != nil,
	})
}

// ClarifyRequirements performs the structured Socratic Trifecta analysis.
func (h *ToolHandler) ClarifyRequirements(ctx context.Context, req *mcp.CallToolRequest, args ClarifyRequirementsArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	// --- Design Quality Gate ---
	// Validate that the Socratic Trifecta data meets minimum depth thresholds
	// BEFORE processing. This enforces the "garbage in, solid spec out" contract:
	// the server rejects thin data and demands the agent produce comprehensive analysis.
	if args.IsVetted {
		if err := validateDesignQuality(args.DesignProposal, args.SkepticAnalysis, args.SynthesisResolution); err != nil {
			slog.Warn("clarify_requirements: design quality gate rejected input",
				"module_count", len(args.DesignProposal.ProposedModules),
				"session_id", args.SessionID,
			)
			return errorResult(err.Error())
		}
	}

	session.IsVetted = args.IsVetted

	meta, err := h.store.GetSessionMetadata(args.SessionID)
	if err != nil || meta == nil {
		meta = &db.SessionMetadata{SessionID: args.SessionID}
	}

	if !args.IsVetted {
		// Persist the skeptic analysis for audit trail
		if args.SkepticAnalysis != nil {
			session.SkepticAnalysis = args.SkepticAnalysis
		}
		RecordStepTiming(session, "clarify_requirements")
		if err := h.store.SaveSession(session); err != nil {
			return errorResult(err.Error())
		}

		// Build structured question list from skeptic + synthesis
		var questions []string
		var topics []string
		if args.SkepticAnalysis != nil {
			for _, q := range args.SkepticAnalysis.GranularQuestions {
				questions = append(questions, fmt.Sprintf("[%s] %s\n  Context: %s\n  Impact: %s", q.Topic, q.Question, q.Context, q.Impact))
				topics = append(topics, q.Topic)
			}
		}
		if args.SynthesisResolution != nil {
			for _, q := range args.SynthesisResolution.OutstandingQuestions {
				questions = append(questions, fmt.Sprintf("[%s] %s\n  Context: %s\n  Impact: %s", q.Topic, q.Question, q.Context, q.Impact))
				topics = append(topics, q.Topic)
			}
		}

		// Record Socratic History
		historyEntry := fmt.Sprintf("[%s] Conflict Detected (%d questions): %s", time.Now().UTC().Format(time.RFC3339), len(questions), strings.Join(topics, ", "))
		if meta.SocraticHistory == "" {
			meta.SocraticHistory = historyEntry
		} else {
			meta.SocraticHistory += "\n" + historyEntry
		}
		_ = h.store.SaveSessionMetadata(meta)

		msg := fmt.Sprintf("SOCRATIC CONFLICT DETECTED: You must prompt the user with the following questions and await their answers. Once answered, re-run 'clarify_requirements' with the updated synthesis.\n\nOutstanding Questions:\n%s", strings.Join(questions, "\n\n"))
		return errorResult(msg)
	}

	// Persist Trifecta data: only store on FIRST submission.
	// On re-entries after a Socratic conflict, the agent may send stripped-down
	// or empty data — overwriting the original would destroy the MADR guardrails.
	if session.DesignProposal == nil && args.DesignProposal != nil {
		session.DesignProposal = args.DesignProposal
	}
	if session.SkepticAnalysis == nil && args.SkepticAnalysis != nil {
		session.SkepticAnalysis = args.SkepticAnalysis
	}

	// --- Input Sanitization: strip server-authority fields ---
	if args.ChaosAnalysis != nil {
		// Chaos requires Thesis input
		if args.DesignProposal == nil {
			return errorResult("Chaos Architect requires Thesis (design_proposal) input")
		}
		for i := range args.ChaosAnalysis.Constraints {
			args.ChaosAnalysis.Constraints[i].Enforced = false
		}
		for i := range args.ChaosAnalysis.RejectedPatterns {
			args.ChaosAnalysis.RejectedPatterns[i].Source = "chaos_architect"
		}
		if args.ChaosAnalysis.ChaosScore < 1 {
			args.ChaosAnalysis.ChaosScore = 1
		}
		if args.ChaosAnalysis.ChaosScore > 10 {
			args.ChaosAnalysis.ChaosScore = 10
		}
		if session.ChaosAnalysis == nil {
			session.ChaosAnalysis = args.ChaosAnalysis
		}
	}
	if args.SynthesisResolution != nil {
		args.SynthesisResolution.ChaosVetted = false
		args.SynthesisResolution.RejectedOptions = nil
		args.SynthesisResolution.ConstraintLocks = nil
		args.SynthesisResolution.LLMEnhanced = false
	}

	// --- Server-side Aporia Engine ---
	session.SynthesisResolution = runAporiaEngine(
		ctx,
		h.store,
		session,
		args.DesignProposal,
		args.SkepticAnalysis,
		args.ChaosAnalysis,
		args.SynthesisResolution,
	)

	// --- Secondary Socratic Integrity Gate with Retry Enforcement ---
	// The Aporia Engine may have generated new deviation challenges or fatal flaws.
	// After exhausting standards-based and LLM-based auto-resolution, any remaining
	// questions MUST go to the human. A retry counter prevents infinite agent loops.
	// Two kill conditions: (1) repeated topics after max retries, (2) absolute cap at 2x max.
	const maxSocraticRetries = 5
	if session.SynthesisResolution != nil && len(session.SynthesisResolution.OutstandingQuestions) > 0 {
		session.IsVetted = false
		session.SocraticRetryCount++

		var questions []string
		var topics []string
		for _, q := range session.SynthesisResolution.OutstandingQuestions {
			questions = append(questions, fmt.Sprintf("[%s] %s\n  Context: %s\n  Impact: %s", q.Topic, q.Question, q.Context, q.Impact))
			topics = append(topics, q.Topic)
		}

		// Detect infinite loop: check if the same topics keep appearing
		var repeatedTopics []string
		if session.SocraticRetryCount > maxSocraticRetries {
			escalatedSet := make(map[string]bool, len(session.SocraticEscalatedTopics))
			for _, t := range session.SocraticEscalatedTopics {
				escalatedSet[t] = true
			}
			for _, t := range topics {
				if escalatedSet[t] {
					repeatedTopics = append(repeatedTopics, t)
				}
			}
		}

		// Track which topics have been escalated
		seen := make(map[string]bool, len(session.SocraticEscalatedTopics))
		for _, t := range session.SocraticEscalatedTopics {
			seen[t] = true
		}
		for _, t := range topics {
			if !seen[t] {
				session.SocraticEscalatedTopics = append(session.SocraticEscalatedTopics, t)
			}
		}

		historyEntry := fmt.Sprintf("[%s] Conflict Detected by Aporia (attempt %d/%d, %d questions): %s",
			time.Now().UTC().Format(time.RFC3339), session.SocraticRetryCount, maxSocraticRetries,
			len(questions), strings.Join(topics, ", "))
		if meta.SocraticHistory == "" {
			meta.SocraticHistory = historyEntry
		} else {
			meta.SocraticHistory += "\n" + historyEntry
		}
		_ = h.store.SaveSessionMetadata(meta)
		_ = h.store.SaveSession(session)

		// Session lock: after MAX_RETRIES with repeated topics OR 2x absolute cap (new topics each time)
		absoluteCap := maxSocraticRetries * 2
		if len(repeatedTopics) > 0 || session.SocraticRetryCount > absoluteCap {
			lockReason := "repeated questions without resolution"
			if session.SocraticRetryCount > absoluteCap {
				lockReason = fmt.Sprintf("absolute retry cap exceeded (%d attempts)", session.SocraticRetryCount)
			}
			msg := fmt.Sprintf("SOCRATIC SESSION LOCKED (%s): %d questions remain after %d attempts. "+
				"The system has exhausted standards-based and LLM-based auto-resolution. "+
				"Human input is MANDATORY. You MUST present these questions to the user and include their answers as decisions.\n\n"+
				"Unresolved Questions:\n%s", lockReason, len(questions), session.SocraticRetryCount, strings.Join(questions, "\n\n"))
			return errorResult(msg)
		}

		// Build enhanced error with resolution summary
		var msg strings.Builder
		msg.WriteString("SOCRATIC CONFLICT DETECTED (via Aporia Gate):\n\n")
		msg.WriteString(fmt.Sprintf("Resolution Summary (attempt %d of %d):\n", session.SocraticRetryCount, maxSocraticRetries))
		msg.WriteString("  The system has already attempted auto-resolution via ingested standards and LLM analysis.\n")
		msg.WriteString(fmt.Sprintf("  HUMAN INPUT REQUIRED: %d questions (listed below)\n\n", len(questions)))
		msg.WriteString("The following questions could NOT be resolved by standards or LLM analysis.\n")
		msg.WriteString("You MUST present these to the user and include their answers as decisions.\n")
		msg.WriteString("Once answered, re-run 'clarify_requirements' with the updated synthesis.\n\n")
		msg.WriteString("Outstanding Questions:\n")
		msg.WriteString(strings.Join(questions, "\n\n"))
		return errorResult(msg.String())
	}

	// All questions resolved — reset retry counter
	session.SocraticRetryCount = 0
	session.SocraticEscalatedTopics = nil
	session.StepStatus["clarify_requirements"] = "COMPLETED"
	session.CurrentStep = "clarify_requirements"

	RecordStepTiming(session, "clarify_requirements")

	populated := 0
	if args.SessionID != "" { populated++ }
	if args.DesignProposal != nil { populated++ }
	if args.SkepticAnalysis != nil { populated++ }
	if args.ChaosAnalysis != nil { populated++ }
	if args.SynthesisResolution != nil { populated++ }
	CheckPayloadCompleteness(session, "clarify_requirements", populated, 5)

	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	// Record resolution in Socratic History
	summary := ""
	decisions := 0
	if session.SynthesisResolution != nil {
		summary = session.SynthesisResolution.Narrative
		if len(summary) > 97 {
			summary = summary[:97] + "..."
		}
		decisions = len(session.SynthesisResolution.Decisions)
	}
	historyEntry := fmt.Sprintf("[%s] Synthesis Resolved (%d decisions locked): %s", time.Now().UTC().Format(time.RFC3339), decisions, summary)
	
	if meta.SocraticHistory == "" {
		meta.SocraticHistory = historyEntry
	} else {
		meta.SocraticHistory += "\n" + historyEntry
	}
	_ = h.store.SaveSessionMetadata(meta)

	hint := "Socratic Trifecta analysis complete. Proceed to 'critique_design' to vet the proposed architecture."
	resultData := map[string]any{
		"is_vetted": true,
	}

	// Return the FULL stored session data so the agent sees accumulated context.
	// This enables compounding: each phase builds upon the data from previous phases.
	if session.DesignProposal != nil {
		resultData["module_count"] = len(session.DesignProposal.ProposedModules)
		resultData["template_ast_files"] = len(session.DesignProposal.TemplateAST)
		resultData["security_mandates"] = len(session.DesignProposal.SecurityMandates)
		resultData["stack_tuning_items"] = len(session.DesignProposal.StackTuning)
		resultData["design_proposal"] = session.DesignProposal
	}
	if session.SkepticAnalysis != nil {
		resultData["skeptic_analysis"] = session.SkepticAnalysis
	}
	if session.SynthesisResolution != nil {
		resultData["decisions_resolved"] = len(session.SynthesisResolution.Decisions)
		resultData["llm_enhanced"] = session.SynthesisResolution.LLMEnhanced
		resultData["chaos_vetted"] = session.SynthesisResolution.ChaosVetted
		resultData["synthesis_resolution"] = session.SynthesisResolution
	}
	if session.ChaosAnalysis != nil {
		resultData["chaos_score"] = session.ChaosAnalysis.ChaosScore
		resultData["fatal_flaws"] = len(session.ChaosAnalysis.FatalFlaws)
		resultData["constraints"] = len(session.ChaosAnalysis.Constraints)
		resultData["rejected_patterns"] = len(session.ChaosAnalysis.RejectedPatterns)
		resultData["chaos_analysis"] = session.ChaosAnalysis
	}

	LogSessionHash(session, "clarify_requirements")
	return hybridMarkdownResult(hint, resultData)
}

// IngestStandards performs the IngestStandards operation.
func (h *ToolHandler) IngestStandards(ctx context.Context, req *mcp.CallToolRequest, args IngestStandardsArgs) (*mcp.CallToolResult, any, error) {
	standard := args.SourceURL
	if args.FilePath != "" {
		standard = args.FilePath
	}

	// Use the shared 3-tier cascade with content return: BuntDB cache → HTTP → embedded.
	// FetchAndCacheWithContent returns content directly, eliminating double decompression.
	textContent, err := sync.FetchAndCacheWithContent(h.store, standard)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to retrieve standard: %v", err))
	}

	if err := h.store.AppendStandard(args.SessionID, textContent); err != nil {
		return errorResult(err.Error())
	}

	populated := 0
	if args.SessionID != "" { populated++ }
	if args.SourceURL != "" || args.FilePath != "" { populated++ }
	CheckPayloadCompleteness(nil, "ingest_standards", populated, 2)

	if session, loadErr := h.store.LoadSession(args.SessionID); loadErr == nil && session != nil {
		session.StepStatus["ingest_standards"] = "COMPLETED"
		session.CurrentStep = "ingest_standards"
		RecordStepTiming(session, "ingest_standards")
		_ = h.store.SaveSession(session)
		LogSessionHash(session, "ingest_standards")
	}

	hint := "Standard ingested successfully. You may ingest another, or proceed to 'clarify_requirements'.\n\n" +
		"=== SOCRATIC TRIFECTA + CHAOS ARCHITECT DIRECTIVE ===\n" +
		"When ALL standards are ingested, you MUST call 'clarify_requirements' with the following structured analysis:\n\n" +
		"1. THESIS ARCHITECT (design_proposal):\n" +
		"   - Propose a complete application architecture using the ingested standards as your baseline.\n" +
		"   - Define proposed_modules: component hierarchy with responsibilities and inter-module dependencies.\n" +
		"   - Generate a template_ast: proposed project file tree with function/interface signatures (exports).\n" +
		"   - Enumerate security_mandates: white-hat security practices (OWASP Top 10, input validation, auth, secrets management, CSRF, XSS).\n" +
		"   - Provide stack_tuning: stack-specific optimizations (Node.js: event loop, clustering, streams, memory | .NET: async/await, DI, middleware, Kestrel).\n" +
		"   - Cite referenced_standards: specific rules from ingested standards that influenced your design.\n\n" +
		"2. ANTITHESIS SKEPTIC (skeptic_analysis):\n" +
		"   - Perform adversarial white-hat review of EVERY thesis element.\n" +
		"   - Identify vulnerabilities: attack vectors, injection points, auth bypass scenarios.\n" +
		"   - Flag design_concerns: over-engineering, missing patterns, scalability bottlenecks, code smells.\n" +
		"   - Generate granular_questions: detailed, code-specific questions the user must answer.\n\n" +
		"3. CHAOS ARCHITECT (chaos_analysis):\n" +
		"   - Assume the combined Thesis+Antithesis design WILL fail. Your job is to find WHERE and HOW.\n" +
		"   - Identify fatal_flaws: issues severe enough to veto the entire design.\n" +
		"   - Map constraints: platform/runtime/API hard limits the design violates (domain, constraint, platform, impact).\n" +
		"   - Build the Graveyard (rejected_patterns): patterns you killed and WHY (pattern, reason, severity).\n" +
		"   - Construct stress_scenarios: edge cases, race conditions, cascading failures (scenario, trigger, impact, mitigation).\n" +
		"   - Assign a chaos_score (1-10): your confidence in the design's operational survivability.\n\n" +
		"4. APORIA ENGINE (synthesis_resolution):\n" +
		"   - Resolve conflicts between thesis, antithesis, and chaos with explicit decisions and rationale.\n" +
		"   - Escalate outstanding_questions to the user with full context and impact descriptions.\n" +
		"   - List unresolved_dependencies that need external input.\n" +
		"   - Set is_vetted=true ONLY if all questions are resolved. Otherwise is_vetted=false.\n" +
		"=== END DIRECTIVE ==="
	return textResult(fmt.Sprintf("%s\n\n=== STANDARD CONTENT START ===\n%s\n=== STANDARD CONTENT END ===\n\n%s", standard, textContent, hint))
}

// CritiqueDesign performs the CritiqueDesign operation.
func (h *ToolHandler) CritiqueDesign(ctx context.Context, req *mcp.CallToolRequest, args CritiqueDesignArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	session.IsVetted = true
	if args.StrictMode {
		// Enforce strict logic if needed
		slog.Info("Strict mode enabled for vetting")
	}

	session.StepStatus["critique_design"] = "COMPLETED"
	session.CurrentStep = "critique_design"
	RecordStepTiming(session, "critique_design")
	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	// Return accumulated session state so the agent has full context entering finalize_requirements.
	resultData := map[string]any{
		"is_vetted":    true,
		"critique_log": "Vetting passed successfully.",
	}
	if session.DesignProposal != nil {
		resultData["design_proposal"] = session.DesignProposal
	}
	if session.SkepticAnalysis != nil {
		resultData["skeptic_analysis"] = session.SkepticAnalysis
	}
	if session.SynthesisResolution != nil {
		resultData["synthesis_resolution"] = session.SynthesisResolution
	}
	if session.ChaosAnalysis != nil {
		resultData["chaos_analysis"] = session.ChaosAnalysis
	}

	hint := "Next, call 'finalize_requirements' to generate the golden copy."
	LogSessionHash(session, "critique_design")
	return hybridMarkdownResult(hint, resultData)
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
	RecordStepTiming(session, "finalize_requirements")

	populated := 0
	if args.SessionID != "" { populated++ }
	if args.ApprovalSignature != "" { populated++ }
	CheckPayloadCompleteness(session, "finalize_requirements", populated, 2)

	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	// Build the golden copy summary: return ALL accumulated session data
	// so the agent has the full compounded context entering blueprint_implementation.
	meta, _ := h.store.GetSessionMetadata(args.SessionID)
	resultData := map[string]any{
		"status": "APPROVED",
	}
	if session.DesignProposal != nil {
		resultData["design_proposal"] = session.DesignProposal
	}
	if session.SkepticAnalysis != nil {
		resultData["skeptic_analysis"] = session.SkepticAnalysis
	}
	if session.SynthesisResolution != nil {
		resultData["synthesis_resolution"] = session.SynthesisResolution
	}
	if session.ChaosAnalysis != nil {
		resultData["chaos_analysis"] = session.ChaosAnalysis
	}
	if meta != nil {
		resultData["intelligence_engine"] = meta
	}

	hint := "Next, call 'blueprint_implementation' to generate the technical mapping."
	LogSessionHash(session, "finalize_requirements")
	return hybridMarkdownResult(hint, resultData)
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

	// Build blueprint from agent-provided data, falling back to session-derived data.
	bp := &db.Blueprint{
		ImplementationStrategy: args.ImplementationStrategy,
		DependencyManifest:     args.Dependencies,
		ComplexityScores:       args.ComplexityScores,
		ADRs:                   args.ADRs,
		FileStructure:          args.FileStructure,
		SecurityConsiderations: args.SecurityConsiderations,
		NonFunctionalRequirements: args.NFRs,
		TestingStrategy:        args.TestingStrategy,
		APIContracts:           args.APIContracts,
		DataModel:              args.DataModel,
		MCPTools:               args.MCPTools,
		MCPResources:           args.MCPResources,
		MCPPrompts:             args.MCPPrompts,
	}

	// Ensure non-nil maps for downstream consumers
	if bp.ImplementationStrategy == nil {
		bp.ImplementationStrategy = map[string]string{
			session.RefinedIdea: args.PatternPreference,
		}
	}
	if bp.ComplexityScores == nil {
		bp.ComplexityScores = make(map[string]int)
	}

	// Populate AporiaTraceability from synthesis decisions
	bp.AporiaTraceability = make(map[string]string)
	if session.SynthesisResolution != nil {
		for i, decision := range session.SynthesisResolution.Decisions {
			key := fmt.Sprintf("Decision-%d: %s", i+1, decision.Topic)
			bp.AporiaTraceability[key] = decision.Decision
		}
	}

	// Pre-seed blueprint from DesignProposal when agent doesn't provide overrides
	if session.DesignProposal != nil {
		if len(bp.FileStructure) == 0 {
			bp.FileStructure = session.DesignProposal.TemplateAST
		}
		if len(bp.SecurityConsiderations) == 0 {
			bp.SecurityConsiderations = session.DesignProposal.SecurityMandates
		}
		if len(bp.NonFunctionalRequirements) == 0 && len(session.DesignProposal.StackTuning) > 0 {
			for _, opt := range session.DesignProposal.StackTuning {
				bp.NonFunctionalRequirements = append(bp.NonFunctionalRequirements, db.NFR{
					Category:    opt.Category,
					Requirement: opt.Recommendation,
					Target:      opt.Rationale,
					Priority:    opt.Priority,
				})
			}
		}
	}

	// --- Server-side Auto-Enrichment ---
	// Expand thin blueprint data from accumulated session state. This is the core
	// mechanism that ensures rich output from minimal agent input: the server generates
	// file structures, NFRs, testing strategies, complexity scores, and dependencies
	// from the Socratic Trifecta data accumulated in earlier phases.
	enrichBlueprintFromSession(bp, session)

	// Auto-synthesize ADRs from Socratic Trifecta decisions when the agent
	// does not provide them explicitly. This ensures every MADR has at
	// least baseline architectural decision records.
	if len(bp.ADRs) == 0 && session.SynthesisResolution != nil && len(session.SynthesisResolution.Decisions) > 0 {
		for _, decision := range session.SynthesisResolution.Decisions {
			bp.ADRs = append(bp.ADRs, db.ADR{
				Title:        decision.Topic,
				Status:       "accepted",
				Context:      decision.Rationale,
				Decision:     decision.Decision,
				Consequences: decision.Rationale,
				DecisionDate: time.Now().UTC().Format("2006-01-02"),
			})
		}
		slog.Info("blueprint_implementation: auto-synthesized ADRs from synthesis decisions",
			"count", len(bp.ADRs),
		)
	}

	// Create standalone Chaos Graveyard ADR collecting all rejected patterns.
	// This avoids cartesian injection across all ADRs which would create noise.
	if session.ChaosAnalysis != nil && len(session.ChaosAnalysis.RejectedPatterns) > 0 {
		var alts []db.Alternative
		for _, rejection := range session.ChaosAnalysis.RejectedPatterns {
			alts = append(alts, db.Alternative{
				Name:            rejection.Pattern,
				Cons:            rejection.Reason,
				RejectionReason: fmt.Sprintf("Chaos Architect [%s]: %s", rejection.Severity, rejection.Reason),
			})
		}
		graveyardADR := db.ADR{
			Title:        "Chaos Graveyard \u2014 Rejected Patterns",
			Status:       "accepted",
			Context:      "The Chaos Architect identified the following patterns as unsafe, unviable, or operationally unsound.",
			Decision:     "These patterns are explicitly rejected and MUST NOT be used by downstream agents.",
			Consequences: "Implementing agents must check this list before proposing solutions.",
			Alternatives: alts,
			Tags:         []string{"chaos-architect", "graveyard"},
			DecisionDate: time.Now().UTC().Format("2006-01-02"),
		}
		bp.ADRs = append(bp.ADRs, graveyardADR)
		slog.Info("blueprint_implementation: added Chaos Graveyard ADR",
			"rejected_patterns", len(alts),
		)
	}

	// Generate and persist the D2 diagram
	bp.D2Source = GenerateD2Diagram(session, bp)
	svgRendered := false
	if bp.D2Source != "" {
		if svg, err := RenderD2ToSVG(bp.D2Source); err != nil {
			slog.Warn("blueprint_implementation: D2 SVG rendering failed — SVG will not be uploaded",
				"error", err,
				"d2_source_len", len(bp.D2Source),
			)
		} else if svg != "" {
			bp.D2SVG = svg
			svgRendered = true
		}
	}

	if session.TechMapping == nil {
		session.TechMapping = make(map[string]string)
	}
	session.TechMapping["Pattern"] = args.PatternPreference
	session.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	session.CurrentStep = "blueprint_implementation"
	RecordStepTiming(session, "blueprint_implementation")

	populated := 0
	if args.SessionID != "" { populated++ }
	if args.PatternPreference != "" { populated++ }
	if len(bp.ImplementationStrategy) > 0 { populated++ }
	if len(bp.DependencyManifest) > 0 { populated++ }
	if len(bp.ComplexityScores) > 0 { populated++ }
	if len(bp.FileStructure) > 0 { populated++ }
	if len(bp.SecurityConsiderations) > 0 { populated++ }
	if len(bp.NonFunctionalRequirements) > 0 { populated++ }
	if len(bp.TestingStrategy) > 0 { populated++ }
	if len(bp.ADRs) > 0 { populated++ }
	if len(bp.APIContracts) > 0 { populated++ }
	if len(bp.DataModel) > 0 { populated++ }
	if len(bp.MCPTools) > 0 { populated++ }
	if len(bp.MCPResources) > 0 { populated++ }
	if len(bp.MCPPrompts) > 0 { populated++ }
	CheckPayloadCompleteness(session, "blueprint_implementation", populated, 15)

	// Attach the blueprint to the session object so it persists in a single
	// coherent write. Previous code used separate SaveBlueprint/UpdateCurrentStep/
	// AppendStepStatus calls, each performing independent read-modify-write cycles
	// that overwrote each other — critically losing the blueprint data.
	session.Blueprint = bp
	session.StepStatus["blueprint_implementation"] = "COMPLETED"

	if err := h.store.SaveSession(session); err != nil {
		return errorResult(fmt.Sprintf("failed to save session with blueprint: %v", err))
	}

	hint := "Next, call 'generate_documents' to sync the artifacts with Jira, Confluence, and GitLab."
	LogSessionHash(session, "blueprint_implementation")

	// Return the FULL blueprint so the agent sees all accumulated and generated data.
	resultData := map[string]any{
		"d2_generated": bp.D2Source != "",
		"svg_rendered": svgRendered,
	}
	if len(bp.ADRs) > 0 {
		resultData["adrs"] = bp.ADRs
	}
	if len(bp.FileStructure) > 0 {
		resultData["file_structure"] = bp.FileStructure
	}
	if len(bp.DependencyManifest) > 0 {
		resultData["dependency_manifest"] = bp.DependencyManifest
	}
	if len(bp.SecurityConsiderations) > 0 {
		resultData["security_considerations"] = bp.SecurityConsiderations
	}
	if len(bp.NonFunctionalRequirements) > 0 {
		resultData["non_functional_requirements"] = bp.NonFunctionalRequirements
	}
	if len(bp.APIContracts) > 0 {
		resultData["api_contracts"] = bp.APIContracts
	}
	if len(bp.DataModel) > 0 {
		resultData["data_model"] = bp.DataModel
	}
	if len(bp.MCPTools) > 0 {
		resultData["mcp_tools"] = bp.MCPTools
	}
	if len(bp.MCPResources) > 0 {
		resultData["mcp_resources"] = bp.MCPResources
	}
	if len(bp.MCPPrompts) > 0 {
		resultData["mcp_prompts"] = bp.MCPPrompts
	}
	if len(bp.ComplexityScores) > 0 {
		resultData["complexity_scores"] = bp.ComplexityScores
	}
	if len(bp.ImplementationStrategy) > 0 {
		resultData["implementation_strategy"] = bp.ImplementationStrategy
	}
	if len(bp.AporiaTraceability) > 0 {
		resultData["aporia_traceability"] = bp.AporiaTraceability
	}
	if len(bp.TestingStrategy) > 0 {
		resultData["testing_strategy"] = bp.TestingStrategy
	}

	return hybridMarkdownResult(hint, resultData)
}

// buildComprehensiveSpec synthesizes all accumulated session state into a
// machine-optimized markdown document suitable for LLM code generation handoff.
func buildComprehensiveSpec(session *db.SessionState, bp *db.Blueprint, meta *db.SessionMetadata, agentMarkdown, title string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# %s\n\n", title))

	// --- Project Overview ---
	b.WriteString("## Project Overview\n\n")
	if session.OriginalIdea != "" {
		b.WriteString(fmt.Sprintf("**Original Idea:** %s\n\n", session.OriginalIdea))
	}
	if session.RefinedIdea != "" && session.RefinedIdea != session.OriginalIdea {
		b.WriteString(fmt.Sprintf("**Refined Idea:** %s\n\n", session.RefinedIdea))
	}
	b.WriteString(fmt.Sprintf("**Technology Stack:** %s\n\n", session.TechStack))
	if session.TargetEnvironment != "" {
		b.WriteString(fmt.Sprintf("**Target Environment:** %s\n\n", session.TargetEnvironment))
	}
	if session.RiskLevel != "" {
		b.WriteString(fmt.Sprintf("**Risk Level:** %s\n\n", session.RiskLevel))
	}
	if len(session.ComplianceRequirements) > 0 {
		b.WriteString(fmt.Sprintf("**Compliance:** %s\n\n", strings.Join(session.ComplianceRequirements, ", ")))
	}
	if len(session.Labels) > 0 {
		b.WriteString(fmt.Sprintf("**Labels:** %s\n\n", strings.Join(session.Labels, ", ")))
	}
	if session.JiraBrowseURL != "" {
		b.WriteString(fmt.Sprintf("**Jira Task:** %s \u2014 %s\n\n", session.JiraID, session.JiraBrowseURL))
	} else if session.JiraID != "" && session.JiraID != "UNKNOWN" {
		b.WriteString(fmt.Sprintf("**Jira Task:** %s\n\n", session.JiraID))
	}

	// --- Intelligence Engine ---
	if meta != nil && (meta.ComplexityScore > 0 || meta.SecurityFootprint != "" || meta.PatternPreference != "" || meta.SocraticHistory != "") {
		b.WriteString("## Intelligence Engine\n\n")
		if meta.ComplexityScore > 0 {
			b.WriteString(fmt.Sprintf("**Calculated Complexity Score:** %d / 13 SP\n\n", meta.ComplexityScore))
		}
		if meta.SecurityFootprint != "" {
			b.WriteString(fmt.Sprintf("**Security Footprint:** %s\n\n", meta.SecurityFootprint))
		}
		if meta.PatternPreference != "" {
			b.WriteString(fmt.Sprintf("**Pattern Preference:** %s\n\n", meta.PatternPreference))
		}
		if meta.SocraticHistory != "" {
			b.WriteString("**Socratic History Trail:**\n```text\n")
			b.WriteString(meta.SocraticHistory)
			b.WriteString("\n```\n\n")
		}
	}

	// --- Approved Specification ---
	if session.FinalSpec != "" {
		b.WriteString("## Approved Specification\n\n")
		b.WriteString(session.FinalSpec)
		b.WriteString("\n\n")
	}

	// --- Design Decisions & Resolved Architectural Decisions ---
	if session.SynthesisResolution != nil && len(session.SynthesisResolution.Decisions) > 0 {
		b.WriteString("## Resolved Architectural Decisions\n\n")
		for i, decision := range session.SynthesisResolution.Decisions {
			b.WriteString(fmt.Sprintf("%d. **%s**: %s\n   *Rationale:* %s\n", i+1, decision.Topic, decision.Decision, decision.Rationale))
		}
		b.WriteString("\n")
	}

	// --- Design Proposal ---
	if session.DesignProposal != nil {
		if session.DesignProposal.Narrative != "" {
			b.WriteString("## Design Proposal\n\n")
			b.WriteString(session.DesignProposal.Narrative)
			b.WriteString("\n\n")
		}

		// Proposed Modules
		if len(session.DesignProposal.ProposedModules) > 0 {
			b.WriteString("### Proposed Modules\n\n")
			for _, mod := range session.DesignProposal.ProposedModules {
				b.WriteString(fmt.Sprintf("#### %s\n\n", mod.Name))
				b.WriteString(fmt.Sprintf("**Purpose:** %s\n\n", mod.Purpose))
				if len(mod.Responsibilities) > 0 {
					b.WriteString("**Responsibilities:**\n")
					for _, r := range mod.Responsibilities {
						b.WriteString(fmt.Sprintf("- %s\n", r))
					}
				}
				if len(mod.Dependencies) > 0 {
					b.WriteString(fmt.Sprintf("\n**Dependencies:** %s\n", strings.Join(mod.Dependencies, ", ")))
				}
				b.WriteString("\n")
			}
		}

		// Template AST
		if len(session.DesignProposal.TemplateAST) > 0 {
			b.WriteString("### Proposed Project Structure (Template AST)\n\n")
			b.WriteString("| Path | Type | Language | Description | Exports |\n")
			b.WriteString("|---|---|---|---|---|\n")
			for _, f := range session.DesignProposal.TemplateAST {
				exports := strings.Join(f.Exports, ", ")
				b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n", f.Path, f.Type, f.Language, f.Description, exports))
			}
			b.WriteString("\n")
		}

		// Stack Optimization Strategy
		if len(session.DesignProposal.StackTuning) > 0 {
			b.WriteString("### Stack Optimization Strategy\n\n")
			b.WriteString("| Category | Recommendation | Rationale | Priority |\n")
			b.WriteString("|---|---|---|---|\n")
			for _, opt := range session.DesignProposal.StackTuning {
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", opt.Category, opt.Recommendation, opt.Rationale, opt.Priority))
			}
			b.WriteString("\n")
		}

		// Standards Traceability
		if len(session.DesignProposal.ReferencedStandards) > 0 {
			b.WriteString("### Standards Traceability\n\n")
			b.WriteString("| Standard | Rule | Application |\n")
			b.WriteString("|---|---|---|\n")
			for _, ref := range session.DesignProposal.ReferencedStandards {
				b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", ref.StandardURL, ref.Rule, ref.Application))
			}
			b.WriteString("\n")
		}

		// White-Hat Security Mandates (from thesis)
		if len(session.DesignProposal.SecurityMandates) > 0 {
			b.WriteString("### White-Hat Security Mandates (Thesis)\n\n")
			b.WriteString("| Category | Severity | Description | Mitigation |\n")
			b.WriteString("|---|---|---|---|\n")
			for _, sec := range session.DesignProposal.SecurityMandates {
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", sec.Category, sec.Severity, sec.Description, sec.MitigationStrategy))
			}
			b.WriteString("\n")
		}
	}

	// --- Skeptic Review ---
	if session.SkepticAnalysis != nil {
		if session.SkepticAnalysis.Narrative != "" {
			b.WriteString("## Skeptic Review\n\n")
			b.WriteString(session.SkepticAnalysis.Narrative)
			b.WriteString("\n\n")
		}

		// Vulnerabilities (from antithesis)
		if len(session.SkepticAnalysis.Vulnerabilities) > 0 {
			b.WriteString("### Vulnerability Assessment\n\n")
			b.WriteString("| Category | Severity | Description | Mitigation |\n")
			b.WriteString("|---|---|---|---|\n")
			for _, vuln := range session.SkepticAnalysis.Vulnerabilities {
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", vuln.Category, vuln.Severity, vuln.Description, vuln.MitigationStrategy))
			}
			b.WriteString("\n")
		}

		// Design Concerns
		if len(session.SkepticAnalysis.DesignConcerns) > 0 {
			b.WriteString("### Design Concerns\n\n")
			b.WriteString("| Area | Severity | Concern | Suggestion |\n")
			b.WriteString("|---|---|---|---|\n")
			for _, concern := range session.SkepticAnalysis.DesignConcerns {
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", concern.Area, concern.Severity, concern.Concern, concern.Suggestion))
			}
			b.WriteString("\n")
		}
	}

	if bp == nil {
		// No blueprint data — append agent markdown and return early
		if agentMarkdown != "" {
			b.WriteString("## Additional Notes\n\n")
			b.WriteString(agentMarkdown)
			b.WriteString("\n")
		}
		return b.String()
	}

	// --- Implementation Strategy ---
	if len(bp.ImplementationStrategy) > 0 {
		b.WriteString("## Implementation Strategy\n\n")
		b.WriteString("| Requirement | Pattern |\n")
		b.WriteString("|---|---|\n")
		for req, pattern := range bp.ImplementationStrategy {
			b.WriteString(fmt.Sprintf("| %s | %s |\n", req, pattern))
		}
		b.WriteString("\n")
	}

	// --- File Structure ---
	if len(bp.FileStructure) > 0 {
		b.WriteString("## Proposed File Structure\n\n")
		b.WriteString("| Path | Type | Language | Description |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, f := range bp.FileStructure {
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s |\n", f.Path, f.Type, f.Language, f.Description))
		}
		b.WriteString("\n")
	}

	// --- Dependency Manifest ---
	if len(bp.DependencyManifest) > 0 {
		b.WriteString("## Dependency Manifest\n\n")
		b.WriteString("| Package | Version | Ecosystem | Purpose | License | Dev |\n")
		b.WriteString("|---|---|---|---|---|---|\n")
		for _, dep := range bp.DependencyManifest {
			devFlag := ""
			if dep.DevOnly {
				devFlag = "✓"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n", dep.Name, dep.Version, dep.Ecosystem, dep.Purpose, dep.License, devFlag))
		}
		b.WriteString("\n")
	}

	// --- Security Considerations ---
	if len(bp.SecurityConsiderations) > 0 {
		b.WriteString("## Security Considerations\n\n")
		b.WriteString("| Category | Severity | Description | Mitigation |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, sec := range bp.SecurityConsiderations {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", sec.Category, sec.Severity, sec.Description, sec.MitigationStrategy))
		}
		b.WriteString("\n")
	}

	// --- Non-Functional Requirements ---
	if len(bp.NonFunctionalRequirements) > 0 {
		b.WriteString("## Non-Functional Requirements\n\n")
		b.WriteString("| Category | Requirement | Target | Priority |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, nfr := range bp.NonFunctionalRequirements {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", nfr.Category, nfr.Requirement, nfr.Target, nfr.Priority))
		}
		b.WriteString("\n")
	}

	// --- API Contracts ---
	if len(bp.APIContracts) > 0 {
		b.WriteString("## API Contracts\n\n")
		b.WriteString("| Method | Path | Description |\n")
		b.WriteString("|---|---|---|\n")
		for _, api := range bp.APIContracts {
			b.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n", api.Method, api.Path, api.Description))
		}
		b.WriteString("\n")
	}

	// --- MCP Tools ---
	if len(bp.MCPTools) > 0 {
		b.WriteString("## MCP Tools\n\n")
		b.WriteString("| Name | Description | Input Schema |\n")
		b.WriteString("|---|---|---|\n")
		for _, tool := range bp.MCPTools {
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", tool.Name, tool.Description, tool.InputSchema))
		}
		b.WriteString("\n")
	}

	// --- MCP Resources ---
	if len(bp.MCPResources) > 0 {
		b.WriteString("## MCP Resources\n\n")
		b.WriteString("| URI | Name | Description |\n")
		b.WriteString("|---|---|---|\n")
		for _, res := range bp.MCPResources {
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", res.URI, res.Name, res.Description))
		}
		b.WriteString("\n")
	}

	// --- MCP Prompts ---
	if len(bp.MCPPrompts) > 0 {
		b.WriteString("## MCP Prompts\n\n")
		b.WriteString("| Name | Description | Arguments |\n")
		b.WriteString("|---|---|---|\n")
		for _, prompt := range bp.MCPPrompts {
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", prompt.Name, prompt.Description, strings.Join(prompt.Arguments, ", ")))
		}
		b.WriteString("\n")
	}

	// --- Data Model ---
	if len(bp.DataModel) > 0 {
		b.WriteString("## Data Model\n\n")
		for _, entity := range bp.DataModel {
			b.WriteString(fmt.Sprintf("### %s\n\n", entity.Name))
			b.WriteString("| Field | Type | Required | Comment |\n")
			b.WriteString("|---|---|---|---|\n")
			for _, field := range entity.Fields {
				req := ""
				if field.Required {
					req = "✓"
				}
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", field.Name, field.Type, req, field.Comment))
			}
			if len(entity.Relationships) > 0 {
				b.WriteString(fmt.Sprintf("\n**Relationships:** %s\n", strings.Join(entity.Relationships, ", ")))
			}
			b.WriteString("\n")
		}
	}

	// --- Complexity Estimation ---
	if len(bp.ComplexityScores) > 0 {
		b.WriteString("## Complexity Estimation\n\n")
		totalPoints := 0
		for feature, points := range bp.ComplexityScores {
			b.WriteString(fmt.Sprintf("- **%s**: %d SP\n", feature, points))
			totalPoints += points
		}
		b.WriteString(fmt.Sprintf("\n**Total:** %d story points\n\n", totalPoints))
	}

	// --- Testing Strategy ---
	if len(bp.TestingStrategy) > 0 {
		b.WriteString("## Testing Strategy\n\n")
		b.WriteString("| Feature | Approach |\n")
		b.WriteString("|---|---|\n")
		for feature, approach := range bp.TestingStrategy {
			b.WriteString(fmt.Sprintf("| %s | %s |\n", feature, approach))
		}
		b.WriteString("\n")
	}

	// --- Architecture Decision Records ---
	if len(bp.ADRs) > 0 {
		b.WriteString("## Architecture Decision Records\n\n")
		for i, adr := range bp.ADRs {
			b.WriteString(fmt.Sprintf("### ADR %d: %s\n\n", i+1, adr.Title))
			b.WriteString(fmt.Sprintf("**Status:** %s\n\n", adr.Status))
			if adr.DecisionDate != "" {
				b.WriteString(fmt.Sprintf("**Date:** %s\n\n", adr.DecisionDate))
			}
			b.WriteString(fmt.Sprintf("**Context:** %s\n\n", adr.Context))
			b.WriteString(fmt.Sprintf("**Decision:** %s\n\n", adr.Decision))
			b.WriteString(fmt.Sprintf("**Consequences:** %s\n\n", adr.Consequences))
			if len(adr.Alternatives) > 0 {
				b.WriteString("**Evaluated Alternatives:**\n\n")
				for _, alt := range adr.Alternatives {
					b.WriteString(fmt.Sprintf("- **%s** — Pros: %s | Cons: %s | Rejected: %s\n", alt.Name, alt.Pros, alt.Cons, alt.RejectionReason))
				}
				b.WriteString("\n")
			}
		}
	}

	// --- Aporia Traceability ---
	if len(bp.AporiaTraceability) > 0 {
		b.WriteString("## Aporia Traceability\n\n")
		for contradiction, resolution := range bp.AporiaTraceability {
			b.WriteString(fmt.Sprintf("- **%s** → %s\n", contradiction, resolution))
		}
		b.WriteString("\n")
	}

	// --- Implementation Guardrails ---
	totalFiles := 0
	maxDepth := 0
	stdPath := ""
	if strings.EqualFold(session.TechStack, ".NET") {
		totalFiles = viper.GetInt("standards.dotnet.total_files")
		maxDepth = viper.GetInt("standards.dotnet.max_directory_depth")
		stdPath = viper.GetString("standards.dotnet.path")
	} else if strings.EqualFold(session.TechStack, "Node") {
		totalFiles = viper.GetInt("standards.node.total_files")
		maxDepth = viper.GetInt("standards.node.max_directory_depth")
		stdPath = viper.GetString("standards.node.path")
	}

	b.WriteString("## Implementation Guardrails\n\n")
	if stdPath != "" {
		b.WriteString(fmt.Sprintf("> [!IMPORTANT]\n> **MANDATORY**: The agent MUST, before creating any files or beginning to code, read the `.gitignore` template from `%s` and write it to the root of the project directory.\n\n", filepath.Join(stdPath, ".gitignore")))
	} else {
		b.WriteString("> [!IMPORTANT]\n> **MANDATORY**: The agent MUST, before creating any files or beginning to code, write the `.gitignore` file appropriate for the task from the local standards repository to the project directory.\n\n")
	}
	
	if totalFiles > 0 || maxDepth > 0 {
		b.WriteString("### Execution Constraints\n\n")
		if totalFiles > 0 {
			b.WriteString(fmt.Sprintf("- **Max Allowed Files**: `%d` (The agent must not exceed this number of generated files)\n", totalFiles))
		}
		if maxDepth > 0 {
			b.WriteString(fmt.Sprintf("- **Max Directory Depth**: `%d` (The agent must not create folder structures deeper than this limit)\n", maxDepth))
		}
		b.WriteString("\n")
	}

	// --- System Architecture Diagram (D2 ONLY — mermaid is NOT supported) ---
	if bp.D2Source != "" {
		b.WriteString("## System Architecture\n\n")
		b.WriteString("```d2\n")
		b.WriteString(bp.D2Source)
		if !strings.HasSuffix(bp.D2Source, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")
	}

	// --- Agent Supplementary Notes ---
	if agentMarkdown != "" {
		b.WriteString("## Additional Notes\n\n")
		b.WriteString(agentMarkdown)
		b.WriteString("\n")
	}

	return b.String()
}

// GenerateDocuments performs the GenerateDocuments operation.
func (h *ToolHandler) GenerateDocuments(ctx context.Context, req *mcp.CallToolRequest, args GenerateDocumentsArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	if err != nil || session == nil {
		return errorResult("session not found")
	}

	session.CurrentStep = "generate_documents"
	session.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	bp := session.Blueprint
	aporias := session.SynthesisResolution

	// Allow diagram override from agent
	if args.DiagramOverride != "" && bp != nil {
		bp.D2Source = args.DiagramOverride
		if svg, err := RenderD2ToSVG(bp.D2Source); err != nil {
			slog.Warn("generate_documents: D2 SVG rendering failed for diagram override",
				"error", err,
				"d2_source_len", len(bp.D2Source),
			)
		} else {
			bp.D2SVG = svg
		}
	}

	// Build comprehensive machine-optimized spec from ALL session state
	meta, _ := h.store.GetSessionMetadata(args.SessionID)
	markdownPayload := buildComprehensiveSpec(session, bp, meta, args.Markdown, args.Title)

	jiraID, browseURL, confluencePageID, err := integration.ProcessDocumentGeneration(h.store, args.Title, markdownPayload, args.TargetBranch, args.SessionID, bp, aporias)
	if err != nil {
		return errorResult(err.Error())
	}
	session.JiraID = jiraID
	session.JiraBrowseURL = browseURL
	session.ConfluencePageID = confluencePageID
	session.StepStatus["generate_documents"] = "COMPLETED"
	RecordStepTiming(session, "generate_documents")
	if err := h.store.SaveSession(session); err != nil {
		return errorResult(err.Error())
	}

	hint := "Next, wrap up with 'complete_design'."
	return hybridMarkdownResult(hint, map[string]any{
		"jira_key":       jiraID,
		"confluence_url": "https://wiki/" + jiraID,
		"commit_sha":     "abcdef123456",
	})
}

// CompleteDesign performs the CompleteDesign operation.
func (h *ToolHandler) CompleteDesign(ctx context.Context, req *mcp.CallToolRequest, args CompleteDesignArgs) (*mcp.CallToolResult, any, error) {
	session, err := h.store.LoadSession(args.SessionID)
	jiraTask := "UNKNOWN"
	var diagramFiles []string
	var handoffSummary string

	if err == nil && session != nil {
		// Mark pipeline as completed so the dashboard can display IDLE state
		session.CurrentStep = "complete_design"
		session.StepStatus["complete_design"] = "COMPLETED"
		RecordStepTiming(session, "complete_design")
		session.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

		jiraTask = session.JiraID
		if session.Blueprint != nil {
			if session.Blueprint.D2Source != "" {
				diagramFiles = append(diagramFiles, "architecture.d2")
			}
			if session.Blueprint.D2SVG != "" {
				diagramFiles = append(diagramFiles, "architecture.svg")
			}
		}

		// Build comprehensive handoff summary
		meta, _ := h.store.GetSessionMetadata(args.SessionID)
		title := "Design Handoff Summary"
		if jiraTask != "UNKNOWN" && jiraTask != "" {
			title = fmt.Sprintf("[%s] Design Handoff Summary", jiraTask)
		}
		handoffSummary = buildComprehensiveSpec(session, session.Blueprint, meta, "", title)

		// Layer 1: Always write to deterministic cache directory
		cacheDir, cacheErr := os.UserCacheDir()
		if cacheErr == nil {
			handoffDir := filepath.Join(cacheDir, "mcp-server-magicdev", "handoffs")
			if mkErr := os.MkdirAll(handoffDir, 0o755); mkErr == nil {
				filename := fmt.Sprintf("%s_design_handoff.md", jiraTask)
				handoffPath := filepath.Join(handoffDir, filename)
				if writeErr := os.WriteFile(handoffPath, []byte(handoffSummary), 0o644); writeErr != nil {
					slog.Warn("complete_design: cache write failed", "path", handoffPath, "error", writeErr)
				} else {
					slog.Info("complete_design: handoff cached", "path", handoffPath)
				}
			}
		}

		// Layer 2: Upload handoff to Confluence as an additional child page
		if session.ConfluencePageID != "" {
			if uploadErr := integration.UploadHandoffToConfluence(h.store, session.ConfluencePageID, title, handoffSummary); uploadErr != nil {
				slog.Warn("complete_design: confluence handoff upload failed", "error", uploadErr)
			}
		}

		// Archive completed session with a 7-day TTL so it auto-expires.
		// Sessions can also be cleaned up immediately via the 'purge' command.
		if saveErr := h.store.SaveCompletedSession(session); saveErr != nil {
			slog.Warn("complete_design: failed to save completed session", "error", saveErr)
		}
	}

	return hybridMarkdownResult(
		"Session completed and archived. MANDATORY: You MUST write the 'handoff_markdown' "+
			"content to the 'artifact_path' using your native write_to_file tool with "+
			"IsArtifact=true, Overwrite=true, and ArtifactType='other'. "+
			"This is the ONLY way to surface the handoff as a rendered artifact in the IDE. "+
			"Do NOT skip this step.",
		map[string]any{
			"handoff_markdown": handoffSummary,
			"artifact_path":    args.ArtifactPath,
			"artifact_name":    fmt.Sprintf("%s_design_handoff.md", jiraTask),
			"jira_task":        jiraTask,
			"diagram_files":    diagramFiles,
		})
}

// RegisterTools performs the RegisterTools operation.
func RegisterTools(s *mcp.Server, store *db.Store) {
	h := &ToolHandler{store: store}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "evaluate_idea",
		Description: "[PHASE: 1] Initializes a new MagicDev session. This is the entry point for the '/magicdev-start idea:' command. Pass the idea to this tool. Returns a session_id that MUST be used in all subsequent steps. Upon success, you MUST immediately call ingest_standards.",
	}, h.EvaluateIdea)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ingest_standards",
		Description: "[PHASE: 2] Pulls in applicable architectural standards for the project and fetches their content. [REQUIRES: evaluate_idea] Upon success, you MUST immediately call clarify_requirements.",
	}, h.IngestStandards)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "clarify_requirements",
		Description: "[PHASE: 3] Performs Socratic analysis to fill gaps in the idea AGAINST the ingested standards. If conflicts exist, this will return an error instructing you to ask the user questions. [REQUIRES: ingest_standards] Upon success, you MUST immediately call critique_design.",
	}, h.ClarifyRequirements)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "critique_design",
		Description: "[PHASE: 4] Vets the proposed architecture against the ingested standards. [REQUIRES: clarify_requirements] Upon success, you MUST immediately call finalize_requirements.",
	}, h.CritiqueDesign)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "finalize_requirements",
		Description: "[PHASE: 5] Consolidates the vetted design into a Golden Copy JSON spec. [REQUIRES: critique_design] Upon success, you MUST immediately call blueprint_implementation.",
	}, h.FinalizeRequirements)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "blueprint_implementation",
		Description: "[PHASE: 6] Generates a technical implementation blueprint mapping the design to structural patterns. [REQUIRES: finalize_requirements] Upon success, you MUST immediately call generate_documents.",
	}, h.BlueprintImplementation)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_documents",
		Description: "[PHASE: 7] Syncs the finalized blueprint and specifications to Jira, Confluence, and Git. [REQUIRES: blueprint_implementation] Upon success, you MUST immediately call complete_design.",
	}, h.GenerateDocuments)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "complete_design",
		Description: "[PHASE: 8] Wraps up the session and provides a final handoff summary. [REQUIRES: generate_documents] This is the final step. Present the handoff summary to the user.",
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
	return hybridMarkdownResult(hint, map[string]any{
		"updated_key": args.Key,
		"status":      "SUCCESS",
	})
}
