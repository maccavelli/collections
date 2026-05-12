// Package db provides BuntDB-backed session persistence for the MagicDev
// pipeline state machine. Each session tracks the lifecycle of a requirements
// engineering workflow from idea evaluation through document generation.
package db

// CurrentSchemaVersion is the active schema revision for SessionState serialization.
// Increment this when adding fields that require migration logic.
const CurrentSchemaVersion = 1

// Dependency represents a single package in the dependency manifest
// produced by the blueprint_implementation tool.
type Dependency struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem"`          // "nuget" or "npm"
	Purpose   string `json:"purpose,omitzero"`   // Why: "HTTP framework", "ORM"
	License   string `json:"license,omitzero"`   // "MIT", "Apache-2.0"
	DevOnly   bool   `json:"dev_only,omitzero"`  // true if devDependency
}

// Alternative represents a rejected option during an architectural decision,
// following AWS ADR best practices for documenting evaluated alternatives.
type Alternative struct {
	Name            string `json:"name"`
	Pros            string `json:"pros"`
	Cons            string `json:"cons"`
	RejectionReason string `json:"rejection_reason"`
}

// ADR represents an Architecture Decision Record (Nygard format),
// extended with alternatives tracking per AWS best practices.
type ADR struct {
	Title             string        `json:"title"`
	Status            string        `json:"status"`
	Context           string        `json:"context"`
	Decision          string        `json:"decision"`
	Consequences      string        `json:"consequences"`
	Alternatives      []Alternative `json:"alternatives,omitzero"` // Rejected options
	DecisionDate      string        `json:"decision_date,omitzero"`
	Supersedes        string        `json:"supersedes,omitzero"`   // ID of superseded ADR
	Tags              []string      `json:"tags,omitzero"`         // "security", "performance"
	DecisionDrivers   []string      `json:"decision_drivers,omitzero"`
	Confirmation      string        `json:"confirmation,omitzero"`
	ComplianceCheck   string        `json:"compliance_check,omitzero"`
	SecurityFootprint string        `json:"security_footprint,omitzero"`
}

// FileEntry represents a single file or directory in the proposed project scaffold.
type FileEntry struct {
	Path        string   `json:"path"`                 // "src/index.ts"
	Type        string   `json:"type"`                 // "file" or "dir"
	Description string   `json:"description,omitzero"` // "Application entry point"
	Language    string   `json:"language,omitzero"`     // "typescript"
	Exports     []string `json:"exports,omitzero"`      // Proposed function/interface signatures for template AST
}

// ModuleSpec represents a proposed application module in the design proposal.
type ModuleSpec struct {
	Name             string   `json:"name"`
	Purpose          string   `json:"purpose"`
	Responsibilities []string `json:"responsibilities"`
	Dependencies     []string `json:"dependencies,omitzero"` // Inter-module dependencies
}

// StackOptimization represents a stack-specific performance tuning recommendation.
type StackOptimization struct {
	Category       string `json:"category"`       // "memory", "concurrency", "io", "startup"
	Recommendation string `json:"recommendation"`
	Rationale      string `json:"rationale"`
	Priority       string `json:"priority"`       // "must-have", "should-have", "nice-to-have"
}

// StandardReference represents a citation of a specific standard rule that influenced the design.
type StandardReference struct {
	StandardURL string `json:"standard_url"`
	Rule        string `json:"rule"`
	Application string `json:"application"` // How the rule applies to this design
}

// DesignConcern represents a skeptic-identified issue in the proposed design.
type DesignConcern struct {
	Area       string `json:"area"`       // "architecture", "security", "performance"
	Concern    string `json:"concern"`
	Severity   string `json:"severity"`   // "low", "medium", "high", "critical"
	Suggestion string `json:"suggestion"`
}

// GranularQuestion represents a detailed, code-specific question for user resolution.
type GranularQuestion struct {
	Topic    string `json:"topic"`
	Question string `json:"question"`
	Context  string `json:"context"` // Why it matters
	Impact   string `json:"impact"`  // What design decision depends on the answer
}

// ArchitecturalDecision represents a resolved design decision from the Aporia Engine.
type ArchitecturalDecision struct {
	Topic     string `json:"topic"`
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
}

// DesignProposal holds the structured output of the Thesis Architect phase.
// It proposes what to code and how to code it, grounded in ingested standards.
type DesignProposal struct {
	Narrative          string              `json:"narrative"`                     // Rich text thesis narrative
	ProposedModules    []ModuleSpec        `json:"proposed_modules"`              // Component hierarchy
	TemplateAST        []FileEntry         `json:"template_ast"`                  // Proposed project structure with function signatures
	SecurityMandates   []SecurityItem      `json:"security_mandates"`             // White-hat security mandates
	StackTuning        []StackOptimization `json:"stack_tuning"`                  // Stack-specific performance tuning
	ReferencedStandards []StandardReference `json:"referenced_standards,omitzero"` // BuntDB standard citations
}

// SkepticAnalysis holds the structured output of the Antithesis Skeptic phase.
// It performs adversarial white-hat review of the thesis.
type SkepticAnalysis struct {
	Narrative        string            `json:"narrative"`                   // Skeptic narrative
	Vulnerabilities  []SecurityItem    `json:"vulnerabilities"`             // Identified attack vectors
	DesignConcerns   []DesignConcern   `json:"design_concerns"`             // Over-engineering, missing patterns
	GranularQuestions []GranularQuestion `json:"granular_questions,omitzero"` // Detailed code-level questions
}

// SynthesisResolution holds the structured output of the Aporia Engine phase.
// It resolves conflicts between thesis, antithesis, and chaos or escalates to the user.
type SynthesisResolution struct {
	Narrative              string                  `json:"narrative"`                        // Synthesis narrative
	Decisions              []ArchitecturalDecision  `json:"decisions"`                        // Resolved items
	OutstandingQuestions   []GranularQuestion       `json:"outstanding_questions,omitzero"`    // Items escalated to user
	UnresolvedDependencies []string                 `json:"unresolved_dependencies,omitzero"` // Structural gaps
	ChaosVetted            bool                    `json:"chaos_vetted,omitzero"`             // Whether Chaos Architect passed
	RejectedOptions        []ChaosRejection        `json:"rejected_options,omitzero"`         // From Chaos Graveyard
	ConstraintLocks        []ChaosConstraint       `json:"constraint_locks,omitzero"`         // Hard operational limits
	LLMEnhanced            bool                    `json:"llm_enhanced,omitzero"`             // Whether LLM was used for synthesis
}

// SecurityItem represents an OWASP-aligned security finding or consideration.
type SecurityItem struct {
	Category           string `json:"category"`            // "auth", "injection", "xss", "crypto"
	Description        string `json:"description"`
	Severity           string `json:"severity"`            // "low", "medium", "high", "critical"
	MitigationStrategy string `json:"mitigation_strategy"`
}

// ChaosConstraint represents a hard operational boundary identified by the Chaos Architect.
type ChaosConstraint struct {
	Domain     string `json:"domain"`                // "filesystem", "network", "memory", "concurrency", "api_limits"
	Constraint string `json:"constraint"`            // The specific limitation
	Platform   string `json:"platform"`              // "windows", "linux", "macos", "all"
	Impact     string `json:"impact"`                // How it affects the design
	Enforced   bool   `json:"enforced,omitzero"`     // SERVER-ONLY: set by constraint filtering, NOT from agent input
}

// ChaosRejection represents a pattern killed by the Chaos Architect (the Graveyard).
type ChaosRejection struct {
	Pattern  string `json:"pattern"`              // The rejected approach
	Reason   string `json:"reason"`               // Why it was killed
	Severity string `json:"severity"`             // "warning", "fatal"
	Source   string `json:"source,omitzero"`      // SERVER-ONLY: "chaos_architect", "buntdb_standard", "llm_synthesis"
}

// StressScenario represents an adversarial edge case identified by the Chaos Architect.
type StressScenario struct {
	Scenario   string `json:"scenario"`              // Description of the failure mode
	Trigger    string `json:"trigger"`               // What causes it
	Impact     string `json:"impact"`                // System-level consequence
	Mitigation string `json:"mitigation,omitzero"`   // Proposed fix (or empty if unresolvable)
}

// ChaosAnalysis holds the structured output of the Chaos Architect phase.
// It performs operational stress testing of the combined Thesis+Antithesis design.
type ChaosAnalysis struct {
	Narrative        string            `json:"narrative"`                    // Chaos narrative
	FatalFlaws       []SecurityItem    `json:"fatal_flaws,omitzero"`         // Veto-worthy issues
	Constraints      []ChaosConstraint `json:"constraints,omitzero"`         // Hard operational limits
	RejectedPatterns []ChaosRejection  `json:"rejected_patterns,omitzero"`   // The Graveyard
	StressScenarios  []StressScenario  `json:"stress_scenarios,omitzero"`    // Edge case analysis
	ChaosScore       int               `json:"chaos_score,omitzero"`         // 1-10 (0 = not scored)
}

// NFR represents a Non-Functional Requirement with a measurable target.
type NFR struct {
	Category    string `json:"category"`  // "performance", "scalability", "availability", "security"
	Requirement string `json:"requirement"`
	Target      string `json:"target"`    // "< 200ms p95 response time"
	Priority    string `json:"priority"`  // "must-have", "should-have", "nice-to-have"
}

// APIEndpoint defines a single REST or GraphQL endpoint contract.
type APIEndpoint struct {
	Method         string `json:"method"`                    // "GET", "POST", "QUERY"
	Path           string `json:"path"`                      // "/api/v1/users"
	Description    string `json:"description,omitzero"`
	RequestSchema  string `json:"request_schema,omitzero"`   // JSON Schema or description
	ResponseSchema string `json:"response_schema,omitzero"`  // JSON Schema or description
}

// MCPTool defines a tool exposed by an MCP server.
type MCPTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema string `json:"input_schema,omitzero"` // JSON Schema for arguments
}

// MCPResource defines a resource exposed by an MCP server.
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitzero"`
}

// MCPPrompt defines a prompt exposed by an MCP server.
type MCPPrompt struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitzero"`
	Arguments   []string `json:"arguments,omitzero"` // List of argument names
}

// EntityField defines a single field within a data model entity.
type EntityField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`               // "string", "int", "uuid", "timestamp"
	Required bool   `json:"required,omitzero"`
	Comment  string `json:"comment,omitzero"`
}

// Entity defines a data model entity for ERD generation.
type Entity struct {
	Name          string        `json:"name"`
	Fields        []EntityField `json:"fields"`
	Relationships []string      `json:"relationships,omitzero"` // "User hasMany Posts"
}

// Stakeholder represents a participant in the design pipeline for audit trails.
type Stakeholder struct {
	Name  string `json:"name"`
	Role  string `json:"role"`            // "requester", "approver", "reviewer"
	Email string `json:"email,omitzero"`
}

// StepTiming captures execution telemetry for a single pipeline phase.
type StepTiming struct {
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at,omitzero"`
	DurationMs  int64  `json:"duration_ms,omitzero"`
}

// DocumentVersion tracks a single iteration of generated document output.
type DocumentVersion struct {
	Version   int    `json:"version"`
	CommitSHA string `json:"commit_sha,omitzero"`
	CreatedAt string `json:"created_at"`
	Branch    string `json:"branch"`
}

// Blueprint holds the technical implementation strategy generated by the
// MCP sampling protocol. It maps finalized requirements to concrete patterns,
// packages, complexity estimates, and contradiction resolutions.
type Blueprint struct {
	ImplementationStrategy   map[string]string `json:"implementation_strategy"`               // requirement → pattern mapping
	DependencyManifest       []Dependency      `json:"dependency_manifest"`
	ComplexityScores         map[string]int    `json:"complexity_scores"`                     // feature → 1-13 story points
	AporiaTraceability       map[string]string `json:"aporia_traceability"`                   // contradiction → resolution pattern
	ADRs                     []ADR             `json:"adrs,omitempty"`
	D2Source                 string            `json:"d2_source,omitzero"`                // D2 diagram source text (version-controllable)
	D2SVG                    string            `json:"d2_svg,omitzero"`                   // Rendered SVG output (self-contained, embeddable)
	FileStructure            []FileEntry       `json:"file_structure,omitzero"`               // Proposed project scaffold
	SecurityConsiderations   []SecurityItem    `json:"security_considerations,omitzero"`      // OWASP-aligned findings
	NonFunctionalRequirements []NFR            `json:"non_functional_requirements,omitzero"`  // Performance/scalability targets
	TestingStrategy          map[string]string `json:"testing_strategy,omitzero"`             // Feature → test approach mapping
	APIContracts             []APIEndpoint     `json:"api_contracts,omitzero"`                // REST/GraphQL endpoint definitions
	DataModel                []Entity          `json:"data_model,omitzero"`                   // Entity definitions for ERD generation
	MCPTools                 []MCPTool         `json:"mcp_tools,omitzero"`                    // Tools exposed by the MCP server
	MCPResources             []MCPResource     `json:"mcp_resources,omitzero"`                // Resources exposed by the MCP server
	MCPPrompts               []MCPPrompt       `json:"mcp_prompts,omitzero"`                  // Prompts exposed by the MCP server
}

// SessionMetadata holds intelligence data spanning the Socratic lifecycle.
type SessionMetadata struct {
	SessionID         string `json:"session_id"`
	ComplexityScore   int    `json:"complexity_score"`
	SecurityFootprint string `json:"security_footprint"`
	SocraticHistory   string `json:"socratic_history"`
	PatternPreference string `json:"pattern_preference"`
}

// SessionState holds the pipeline state for a given session. Each field
// corresponds to data produced or consumed by one or more tools in the
// idea-to-asset pipeline.
type SessionState struct {
	SessionID              string                `json:"session_id"`
	SchemaVersion          int                   `json:"schema_version"`
	TechStack              string                `json:"tech_stack"`
	BusinessCase           string                `json:"business_case,omitzero"` // Captures Decision Drivers
	OriginalIdea           string                `json:"original_idea,omitzero"`
	RefinedIdea            string                `json:"refined_idea,omitzero"`
	Standards              []string              `json:"standards,omitzero"`                  // dynamically ingested rules
	StepStatus             map[string]string     `json:"step_status"`                         // tracking progress
	CurrentStep            string                `json:"current_step,omitzero"`
	FinalSpec              string                `json:"final_spec,omitzero"`                 // Golden Copy from finalize_requirements
	DesignProposal         *DesignProposal       `json:"design_proposal,omitzero"`            // Structured thesis from clarify_requirements
	SkepticAnalysis        *SkepticAnalysis       `json:"skeptic_analysis,omitzero"`            // Structured antithesis from clarify_requirements
	ChaosAnalysis          *ChaosAnalysis         `json:"chaos_analysis,omitzero"`              // Structured chaos analysis from clarify_requirements
	SynthesisResolution    *SynthesisResolution   `json:"synthesis_resolution,omitzero"`        // Structured aporia resolution from clarify_requirements
	Blueprint              *Blueprint            `json:"blueprint,omitzero"`
	IsVetted               bool                  `json:"is_vetted,omitzero"`
	StandardsSnapshot      string                `json:"standards_snapshot,omitzero"`
	TechMapping            map[string]string     `json:"tech_mapping,omitzero"`
	JiraID                 string                `json:"jira_id,omitzero"`
	JiraBrowseURL          string                `json:"jira_browse_url,omitzero"`
	ConfluencePageID       string                `json:"confluence_page_id,omitzero"` // Parent page ID from generate_documents
	CreatedAt              string                `json:"created_at,omitzero"`
	UpdatedAt              string                `json:"updated_at,omitzero"`
	Tags                   map[string]string     `json:"tags,omitzero"`                       // Freeform key-value categorization
	Labels                 []string              `json:"labels,omitzero"`                     // Filterable classification labels
	ParentSessionID        string                `json:"parent_session_id,omitzero"`           // Session lineage
	Stakeholders           []Stakeholder         `json:"stakeholders,omitzero"`                // Audit trail participants
	RiskLevel              string                `json:"risk_level,omitzero"`                  // low/medium/high/critical
	ComplianceRequirements []string              `json:"compliance_requirements,omitzero"`     // SOC2, HIPAA, PCI-DSS, GDPR
	TargetEnvironment      string                `json:"target_environment,omitzero"`          // cloud/on-prem/hybrid/edge
	EstimatedEffort        string                `json:"estimated_effort,omitzero"`            // Duration estimate
	StepTimings            map[string]StepTiming `json:"step_timings,omitzero"`                // Per-phase execution telemetry
	StepTokens             map[string]int        `json:"step_tokens,omitzero"`                 // Token delta consumed per phase
	StepDataBytes          map[string]int        `json:"step_data_bytes,omitzero"`             // Bytes inserted per phase
	StepHydrations         map[string]float64    `json:"step_hydrations,omitzero"`             // Payload completeness ratio per phase (0.0-1.0)
	DocumentVersions       []DocumentVersion     `json:"document_versions,omitzero"`           // Iteration tracking
	SocraticRetryCount     int                   `json:"socratic_retry_count,omitzero"`         // Tracks clarify_requirements re-entries for loop detection
	SocraticEscalatedTopics []string             `json:"socratic_escalated_topics,omitzero"`    // Question topics surfaced to agent for loop detection
}

// NewSessionState initializes a fresh session with zeroed collections
// to prevent nil-map panics during downstream tool execution.
func NewSessionState(sessionID string) *SessionState {
	return &SessionState{
		SessionID:      sessionID,
		SchemaVersion:  CurrentSchemaVersion,
		StepStatus:     make(map[string]string),
		TechMapping:    make(map[string]string),
		StepTimings:    make(map[string]StepTiming),
		StepTokens:     make(map[string]int),
		StepDataBytes:  make(map[string]int),
		StepHydrations: make(map[string]float64),
	}
}
