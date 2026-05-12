// Package handler provides MCP tool handlers for the MagicDev pipeline.
package handler

import (
	"fmt"
	"log/slog"
	"strings"

	"mcp-server-magicdev/internal/db"
)

// ----- Input Quality Gate (clarify_requirements) -----

// designProposalQualityThresholds defines the minimum data density expected
// from a Socratic Trifecta submission. If the agent's data falls below these
// thresholds, the server rejects the input with specific remediation guidance.
var designProposalQualityThresholds = struct {
	MinModules              int
	MinResponsibilitiesPer  int
	MinFileEntries          int
	MinSecurityMandates     int
	MinStackTuning          int
	MinSkepticVulns         int
	MinDesignConcerns       int
	MinSynthesisDecisions   int
}{
	MinModules:             3,
	MinResponsibilitiesPer: 3,
	MinFileEntries:         3,
	MinSecurityMandates:    2,
	MinStackTuning:         2,
	MinSkepticVulns:        1,
	MinDesignConcerns:      2,
	MinSynthesisDecisions:  2,
}

// validateDesignQuality enforces minimum data density for the Socratic Trifecta input.
// Returns nil if the data meets quality thresholds. Returns a structured error
// message with specific remediation guidance if the data is insufficient.
func validateDesignQuality(
	proposal *db.DesignProposal,
	skeptic *db.SkepticAnalysis,
	synthesis *db.SynthesisResolution,
) error {
	t := designProposalQualityThresholds
	var deficiencies []string

	// --- Thesis Architect (DesignProposal) Checks ---
	if proposal == nil {
		return fmt.Errorf("design_proposal is required — provide the Thesis Architect's structured analysis")
	}
	if len(proposal.ProposedModules) < t.MinModules {
		deficiencies = append(deficiencies,
			fmt.Sprintf("THESIS: proposed_modules has %d modules (minimum %d). Decompose the system into distinct components with clear boundaries — think about ingestion, processing, storage, API, and presentation layers.",
				len(proposal.ProposedModules), t.MinModules))
	}
	for i, mod := range proposal.ProposedModules {
		if len(mod.Responsibilities) < t.MinResponsibilitiesPer {
			deficiencies = append(deficiencies,
				fmt.Sprintf("THESIS: module '%s' (#%d) has %d responsibilities (minimum %d). Each module should have concrete, testable responsibilities — not just its name restated.",
					mod.Name, i+1, len(mod.Responsibilities), t.MinResponsibilitiesPer))
		}
	}
	if len(proposal.TemplateAST) < t.MinFileEntries {
		deficiencies = append(deficiencies,
			fmt.Sprintf("THESIS: template_ast has %d file entries (minimum %d). Map each module to its source files, plus shared types, config, and entry point files.",
				len(proposal.TemplateAST), t.MinFileEntries))
	}
	if len(proposal.SecurityMandates) < t.MinSecurityMandates {
		deficiencies = append(deficiencies,
			fmt.Sprintf("THESIS: security_mandates has %d items (minimum %d). Address OWASP Top 10 categories relevant to this stack: "+
				"injection (SQLi/NoSQLi/command), broken-authentication (JWT/session), sensitive-data-exposure (encryption at rest/in transit), "+
				"broken-access-control (RBAC/ABAC), security-misconfiguration (headers/CORS), XSS (input sanitization), "+
				"path-traversal (file access sandboxing), resource-exhaustion (memory limits/timeouts).",
				len(proposal.SecurityMandates), t.MinSecurityMandates))
	}
	if len(proposal.StackTuning) < t.MinStackTuning {
		deficiencies = append(deficiencies,
			fmt.Sprintf("THESIS: stack_tuning has %d items (minimum %d). Provide MEASURABLE stack-specific targets. "+
				"For Node: What is the P99 latency target (50ms/100ms/200ms)? Memory ceiling (256MB/512MB/1GB)? Cold start budget (200ms/500ms/1s)? "+
				"For .NET: Async-first IO percentage target (100%%)? DI scope management strategy? Kestrel connection limit? "+
				"For Go: GOMAXPROCS target? Context timeout budget? Error wrapping standard (%%w/sentinel/custom)?",
				len(proposal.StackTuning), t.MinStackTuning))
	}
	if proposal.Narrative == "" {
		deficiencies = append(deficiencies,
			"THESIS: narrative is empty. Provide a rich architectural overview explaining how the modules interact, data flows, and key design decisions.")
	}

	// --- Antithesis Skeptic Checks ---
	if skeptic == nil {
		deficiencies = append(deficiencies,
			"ANTITHESIS: skeptic_analysis is required. Perform adversarial review: identify vulnerabilities, design concerns, and generate code-specific questions.")
	} else {
		if len(skeptic.Vulnerabilities) < t.MinSkepticVulns {
			deficiencies = append(deficiencies,
				fmt.Sprintf("ANTITHESIS: vulnerabilities has %d items (minimum %d). Identify attack vectors, injection points, and auth bypass scenarios.",
					len(skeptic.Vulnerabilities), t.MinSkepticVulns))
		}
		if len(skeptic.DesignConcerns) < t.MinDesignConcerns {
			deficiencies = append(deficiencies,
				fmt.Sprintf("ANTITHESIS: design_concerns has %d items (minimum %d). Flag over-engineering, missing patterns, scalability bottlenecks, and anti-patterns.",
					len(skeptic.DesignConcerns), t.MinDesignConcerns))
		}
		if skeptic.Narrative == "" {
			deficiencies = append(deficiencies,
				"ANTITHESIS: narrative is empty. Summarize the adversarial findings and their systemic implications.")
		}
	}

	// --- Synthesis Resolution Checks ---
	if synthesis == nil {
		deficiencies = append(deficiencies,
			"SYNTHESIS: synthesis_resolution is required. Resolve conflicts between thesis and antithesis with explicit decisions and rationale.")
	} else if len(synthesis.Decisions) < t.MinSynthesisDecisions {
		deficiencies = append(deficiencies,
			fmt.Sprintf("SYNTHESIS: decisions has %d items (minimum %d). Each conflict between thesis and antithesis must be resolved with a decision, topic, and rationale.",
				len(synthesis.Decisions), t.MinSynthesisDecisions))
	}

	if len(deficiencies) > 0 {
		return fmt.Errorf("DESIGN QUALITY GATE FAILED — Your analysis does not meet the minimum depth required for a production specification.\n\n"+
			"Deficiencies (%d):\n• %s\n\n"+
			"ACTION: Enrich your Socratic Trifecta analysis using the ingested standards as your source material. "+
			"Re-read the standard content provided in the ingest_standards response, then re-call clarify_requirements "+
			"with comprehensive analysis that addresses ALL deficiencies above.\n\n"+
			"REMINDER: The Socratic Trifecta directive (provided in the ingest_standards response) contains detailed "+
			"instructions for generating each component. Follow it exhaustively.",
			len(deficiencies), strings.Join(deficiencies, "\n• "))
	}
	return nil
}

// ----- Blueprint Auto-Enrichment (blueprint_implementation) -----

// enrichBlueprintFromSession expands thin blueprint data using accumulated session state.
// This ensures that even when the agent provides minimal blueprint args, the output
// document has comprehensive file structures, NFRs, testing strategies, and ADRs.
func enrichBlueprintFromSession(bp *db.Blueprint, session *db.SessionState) {
	if session.DesignProposal == nil {
		return
	}

	enrichFileStructure(bp, session)
	enrichNFRs(bp, session)
	enrichTestingStrategy(bp, session)
	enrichComplexityScores(bp, session)
	enrichDependencies(bp, session)
	enrichADRsWithAlternatives(bp, session)

	// WP-1A: Structural Constraint Linter — keyword scanning from OWASP Top 10
	enrichSecurityFromKeywords(bp, session)

	// WP-1B: Environment-specific constraint injection
	enrichEnvironmentConstraints(bp, session)

	// WP-1C: Standard-derived ADR content
	enrichADRsFromStandards(bp, session)

	slog.Info("blueprint_implementation: auto-enrichment completed",
		"files", len(bp.FileStructure),
		"nfrs", len(bp.NonFunctionalRequirements),
		"testing_features", len(bp.TestingStrategy),
		"complexity_features", len(bp.ComplexityScores),
		"dependencies", len(bp.DependencyManifest),
		"adrs", len(bp.ADRs),
		"security_items", len(bp.SecurityConsiderations),
	)
}

// enrichFileStructure generates a comprehensive file tree from proposed modules
// when the agent hasn't provided one. Creates one source file per module plus
// standard project files (types, config, entry point, test fixtures).
func enrichFileStructure(bp *db.Blueprint, session *db.SessionState) {
	if len(bp.FileStructure) >= len(session.DesignProposal.ProposedModules) {
		return // Agent provided sufficient file entries
	}

	stack := strings.ToLower(session.TechStack)
	ext := ".ts"
	lang := "TypeScript"
	srcDir := "src"

	switch {
	case strings.Contains(stack, ".net") || strings.Contains(stack, "dotnet") || strings.Contains(stack, "csharp"):
		ext = ".cs"
		lang = "C#"
		srcDir = "src"
	case strings.Contains(stack, "go") || strings.Contains(stack, "golang"):
		ext = ".go"
		lang = "Go"
		srcDir = "internal"
	case strings.Contains(stack, "python"):
		ext = ".py"
		lang = "Python"
		srcDir = "src"
	}

	var files []db.FileEntry

	// Entry point
	entryFile := fmt.Sprintf("%s/index%s", srcDir, ext)
	if ext == ".go" {
		entryFile = "cmd/main.go"
	}
	files = append(files, db.FileEntry{
		Path:        entryFile,
		Type:        "source",
		Language:    lang,
		Description: "Application entry point and bootstrap",
		Exports:     []string{"main"},
	})

	// One file per module
	for _, mod := range session.DesignProposal.ProposedModules {
		moduleName := strings.ToLower(strings.ReplaceAll(mod.Name, " ", "_"))
		filePath := fmt.Sprintf("%s/%s%s", srcDir, moduleName, ext)
		if ext == ".go" {
			filePath = fmt.Sprintf("internal/%s/%s.go", moduleName, moduleName)
		}

		var exports []string
		exports = append(exports, mod.Name)
		for _, resp := range mod.Responsibilities {
			// Extract likely function names from responsibilities
			words := strings.Fields(resp)
			if len(words) >= 2 {
				exports = append(exports, strings.Title(words[0])+strings.Title(words[1]))
			}
		}

		files = append(files, db.FileEntry{
			Path:        filePath,
			Type:        "source",
			Language:    lang,
			Description: mod.Purpose,
			Exports:     exports,
		})
	}

	// Shared types file
	typesFile := fmt.Sprintf("%s/types%s", srcDir, ext)
	if ext == ".go" {
		typesFile = "internal/types/types.go"
	}
	files = append(files, db.FileEntry{
		Path:        typesFile,
		Type:        "source",
		Language:    lang,
		Description: "Shared types, interfaces, and enums",
	})

	// Config files
	switch {
	case ext == ".ts":
		files = append(files,
			db.FileEntry{Path: "tsconfig.json", Type: "config", Description: "TypeScript compiler configuration"},
			db.FileEntry{Path: "package.json", Type: "config", Description: "Node.js package manifest"},
		)
	case ext == ".go":
		files = append(files,
			db.FileEntry{Path: "go.mod", Type: "config", Description: "Go module definition"},
			db.FileEntry{Path: "Makefile", Type: "config", Description: "Build automation"},
		)
	case ext == ".cs":
		files = append(files,
			db.FileEntry{Path: fmt.Sprintf("%s.csproj", session.RefinedIdea), Type: "config", Description: ".NET project file"},
		)
	}

	bp.FileStructure = files
	slog.Info("blueprint_implementation: auto-generated file structure from modules",
		"module_count", len(session.DesignProposal.ProposedModules),
		"file_count", len(files),
	)
}

// enrichNFRs auto-generates baseline non-functional requirements from the stack and
// security mandates when the agent hasn't provided them.
func enrichNFRs(bp *db.Blueprint, session *db.SessionState) {
	if len(bp.NonFunctionalRequirements) >= 3 {
		return // Agent provided sufficient NFRs
	}

	// Generate from stack tuning recommendations (already done in BlueprintImplementation pre-seed)
	// Add baseline NFRs that apply to any application
	baselineNFRs := []db.NFR{
		{Category: "performance", Requirement: "API response latency", Target: "< 200ms p95 under normal load", Priority: "must-have"},
		{Category: "stability", Requirement: "Graceful shutdown", Target: "Clean shutdown on SIGTERM/SIGINT within 5 seconds", Priority: "must-have"},
		{Category: "security", Requirement: "Secrets management", Target: "Zero hardcoded credentials; all secrets from environment or vault", Priority: "must-have"},
	}

	stack := strings.ToLower(session.TechStack)
	switch {
	case strings.Contains(stack, "node"):
		baselineNFRs = append(baselineNFRs,
			db.NFR{Category: "performance", Requirement: "Cold start time", Target: "Server ready within 500ms", Priority: "must-have"},
			db.NFR{Category: "stability", Requirement: "Memory ceiling", Target: "Memory watchdog prevents OOM crashes", Priority: "should-have"},
			db.NFR{Category: "security", Requirement: "Zero shell invocations", Target: "No child_process usage in production code", Priority: "should-have"},
		)
	case strings.Contains(stack, ".net") || strings.Contains(stack, "dotnet"):
		baselineNFRs = append(baselineNFRs,
			db.NFR{Category: "performance", Requirement: "Async-first IO", Target: "All I/O operations use async/await patterns", Priority: "must-have"},
			db.NFR{Category: "stability", Requirement: "DI container lifecycle", Target: "Proper scope management for all injected services", Priority: "should-have"},
		)
	case strings.Contains(stack, "go"):
		baselineNFRs = append(baselineNFRs,
			db.NFR{Category: "performance", Requirement: "Context propagation", Target: "All long-running operations accept context.Context", Priority: "must-have"},
			db.NFR{Category: "stability", Requirement: "Error wrapping", Target: "All errors wrapped with %w for proper error chains", Priority: "must-have"},
		)
	}

	// Merge: don't duplicate existing categories
	existing := make(map[string]bool)
	for _, nfr := range bp.NonFunctionalRequirements {
		existing[nfr.Category+"|"+nfr.Requirement] = true
	}
	for _, nfr := range baselineNFRs {
		if !existing[nfr.Category+"|"+nfr.Requirement] {
			bp.NonFunctionalRequirements = append(bp.NonFunctionalRequirements, nfr)
		}
	}
}

// enrichTestingStrategy auto-generates a testing strategy from the module hierarchy
// when the agent hasn't provided one.
func enrichTestingStrategy(bp *db.Blueprint, session *db.SessionState) {
	if len(bp.TestingStrategy) >= len(session.DesignProposal.ProposedModules) {
		return // Agent provided sufficient test strategies
	}

	if bp.TestingStrategy == nil {
		bp.TestingStrategy = make(map[string]string)
	}

	for _, mod := range session.DesignProposal.ProposedModules {
		if _, exists := bp.TestingStrategy[mod.Name]; exists {
			continue
		}

		// Generate a test approach from the module's responsibilities
		var testApproach string
		if len(mod.Responsibilities) > 0 {
			approaches := make([]string, 0, len(mod.Responsibilities))
			for _, resp := range mod.Responsibilities {
				approaches = append(approaches, fmt.Sprintf("test %s", strings.ToLower(resp)))
			}
			testApproach = fmt.Sprintf("Unit tests: %s", strings.Join(approaches, ", "))
		} else {
			testApproach = fmt.Sprintf("Unit tests for %s core functionality", mod.Name)
		}

		// Add integration test hint if module has dependencies
		if len(mod.Dependencies) > 0 {
			testApproach += fmt.Sprintf("; integration tests with %s", strings.Join(mod.Dependencies, ", "))
		}

		bp.TestingStrategy[mod.Name] = testApproach
	}
}

// enrichComplexityScores auto-generates story point estimates from module responsibilities
// when the agent hasn't provided them.
func enrichComplexityScores(bp *db.Blueprint, session *db.SessionState) {
	if len(bp.ComplexityScores) >= len(session.DesignProposal.ProposedModules) {
		return // Agent provided sufficient scores
	}

	if bp.ComplexityScores == nil {
		bp.ComplexityScores = make(map[string]int)
	}

	for _, mod := range session.DesignProposal.ProposedModules {
		if _, exists := bp.ComplexityScores[mod.Name]; exists {
			continue
		}

		// Estimate complexity: 1 SP base + 1 SP per responsibility + 1 SP per dependency
		score := 1 + len(mod.Responsibilities) + len(mod.Dependencies)
		if score > 13 {
			score = 13 // Cap at 13 SP
		}
		bp.ComplexityScores[mod.Name] = score
	}
}

// enrichDependencies auto-generates a baseline dependency manifest from the
// tech stack when the agent hasn't provided one.
func enrichDependencies(bp *db.Blueprint, session *db.SessionState) {
	if len(bp.DependencyManifest) >= 2 {
		return // Agent provided sufficient deps
	}

	stack := strings.ToLower(session.TechStack)

	var deps []db.Dependency
	switch {
	case strings.Contains(stack, "node"):
		deps = []db.Dependency{
			{Name: "typescript", Version: "^5.7.0", Ecosystem: "npm", Purpose: "TypeScript compiler and type system"},
		}
		// Check if it's an MCP server
		idea := strings.ToLower(session.RefinedIdea + " " + session.OriginalIdea)
		if strings.Contains(idea, "mcp") {
			deps = append(deps,
				db.Dependency{Name: "@modelcontextprotocol/sdk", Version: "^1.12.0", Ecosystem: "npm", Purpose: "MCP server transport and tool registration"},
				db.Dependency{Name: "zod", Version: "^3.24.0", Ecosystem: "npm", Purpose: "Runtime schema validation for MCP tool inputs"},
			)
		}
		// Add common dev deps
		deps = append(deps,
			db.Dependency{Name: "vitest", Version: "^3.1.0", Ecosystem: "npm", Purpose: "Unit and integration testing", DevOnly: true},
			db.Dependency{Name: "tsx", Version: "^4.19.0", Ecosystem: "npm", Purpose: "TypeScript execution for development", DevOnly: true},
		)

	case strings.Contains(stack, ".net") || strings.Contains(stack, "dotnet"):
		deps = []db.Dependency{
			{Name: "Microsoft.Extensions.DependencyInjection", Version: "9.0.0", Ecosystem: "nuget", Purpose: "Dependency injection container"},
			{Name: "Microsoft.Extensions.Logging", Version: "9.0.0", Ecosystem: "nuget", Purpose: "Structured logging abstraction"},
			{Name: "xunit", Version: "2.9.0", Ecosystem: "nuget", Purpose: "Unit testing framework", DevOnly: true},
		}
	}

	// Merge without duplicates
	existing := make(map[string]bool)
	for _, d := range bp.DependencyManifest {
		existing[strings.ToLower(d.Name)] = true
	}
	for _, d := range deps {
		if !existing[strings.ToLower(d.Name)] {
			bp.DependencyManifest = append(bp.DependencyManifest, d)
		}
	}
}

// enrichADRsWithAlternatives expands thin ADRs (those auto-synthesized from synthesis
// decisions) by adding decision drivers, confirmation criteria, and — when the
// SkepticAnalysis contains relevant concerns — rejected alternatives derived from
// the adversarial analysis.
func enrichADRsWithAlternatives(bp *db.Blueprint, session *db.SessionState) {
	if session.SynthesisResolution == nil {
		return
	}

	// Build a lookup of skeptic concerns by area for matching to ADR topics
	var skepticConcernsByArea map[string][]db.DesignConcern
	if session.SkepticAnalysis != nil && len(session.SkepticAnalysis.DesignConcerns) > 0 {
		skepticConcernsByArea = make(map[string][]db.DesignConcern)
		for _, concern := range session.SkepticAnalysis.DesignConcerns {
			key := strings.ToLower(concern.Area)
			skepticConcernsByArea[key] = append(skepticConcernsByArea[key], concern)
		}
	}

	for i := range bp.ADRs {
		adr := &bp.ADRs[i]

		// Skip ADRs that already have alternatives (agent-provided or fully expanded)
		if len(adr.Alternatives) > 0 {
			continue
		}

		// Add decision drivers from synthesis decisions
		if len(adr.DecisionDrivers) == 0 {
			for _, dec := range session.SynthesisResolution.Decisions {
				if strings.EqualFold(dec.Topic, adr.Title) {
					adr.DecisionDrivers = append(adr.DecisionDrivers, dec.Rationale)
				}
			}
			// Add any stack tuning items as drivers
			if session.DesignProposal != nil {
				for _, st := range session.DesignProposal.StackTuning {
					if strings.Contains(strings.ToLower(adr.Title), strings.ToLower(st.Category)) {
						adr.DecisionDrivers = append(adr.DecisionDrivers, fmt.Sprintf("[%s] %s", st.Priority, st.Recommendation))
					}
				}
			}
		}

		// Add confirmation criteria if missing
		if adr.Confirmation == "" {
			adr.Confirmation = fmt.Sprintf("Verify that %s is implemented correctly and passes validation tests", adr.Decision)
		}

		// Generate alternatives from skeptic concerns when they match this ADR's domain
		if skepticConcernsByArea != nil {
			titleLower := strings.ToLower(adr.Title)
			for area, concerns := range skepticConcernsByArea {
				if strings.Contains(titleLower, area) || strings.Contains(area, titleLower) {
					for _, concern := range concerns {
						adr.Alternatives = append(adr.Alternatives, db.Alternative{
							Name:            fmt.Sprintf("Alternative: %s", concern.Suggestion),
							Pros:            concern.Suggestion,
							Cons:            concern.Concern,
							RejectionReason: fmt.Sprintf("Antithesis Skeptic [%s]: %s", concern.Severity, concern.Concern),
						})
					}
				}
			}
		}
	}
}

// ----- WP-1A: Structural Constraint Linter (OWASP Top 10) -----

// securityKeywordRule maps compound keywords to an auto-injectable SecurityItem.
// ALL keywords in a rule must match (case-insensitive) in the combined session text
// for the rule to fire. This prevents false positives from single-word matches.
type securityKeywordRule struct {
	// Keywords are compound — ALL must appear in the scanned text for the rule to fire.
	Keywords []string
	Item     db.SecurityItem
}

// owaspKeywordRules maps OWASP Top 10 categories to compound keyword triggers.
// These are the deterministic backbone of the Structural Constraint Linter.
var owaspKeywordRules = []securityKeywordRule{
	// 1. Injection (SQLi, NoSQLi, Command)
	{Keywords: []string{"database", "query"}, Item: db.SecurityItem{
		Category: "injection", Description: "SQL/NoSQL injection prevention required",
		Severity: "critical", MitigationStrategy: "Use parameterized queries or ORM-native query builders exclusively; never concatenate user input into query strings",
	}},
	{Keywords: []string{"command", "exec"}, Item: db.SecurityItem{
		Category: "injection", Description: "Command injection prevention required",
		Severity: "critical", MitigationStrategy: "Avoid shell invocations entirely; use language-native APIs instead of child_process/exec patterns",
	}},

	// 2. Broken Authentication
	{Keywords: []string{"auth", "token"}, Item: db.SecurityItem{
		Category: "broken-auth", Description: "Authentication token validation required",
		Severity: "high", MitigationStrategy: "Implement JWT/OAuth2 middleware with expiry, issuer, and audience validation; use constant-time comparison for secrets",
	}},
	{Keywords: []string{"session", "login"}, Item: db.SecurityItem{
		Category: "broken-auth", Description: "Session management hardening required",
		Severity: "high", MitigationStrategy: "Use secure, HttpOnly, SameSite cookies; implement session timeout and rotation; invalidate on logout",
	}},

	// 3. Sensitive Data Exposure
	{Keywords: []string{"encrypt", "secret"}, Item: db.SecurityItem{
		Category: "sensitive-data", Description: "Secrets management and encryption required",
		Severity: "critical", MitigationStrategy: "Zero hardcoded credentials; all secrets from environment variables or vault; encrypt sensitive data at rest with AES-256",
	}},
	{Keywords: []string{"password", "hash"}, Item: db.SecurityItem{
		Category: "sensitive-data", Description: "Password hashing required",
		Severity: "critical", MitigationStrategy: "Use bcrypt/scrypt/argon2 with work factor >= 12; never store plaintext or MD5/SHA1 password hashes",
	}},

	// 4. Broken Access Control
	{Keywords: []string{"role", "permission"}, Item: db.SecurityItem{
		Category: "broken-access", Description: "Role-based access control required",
		Severity: "high", MitigationStrategy: "Implement RBAC/ABAC middleware; deny by default; validate permissions on every protected endpoint",
	}},

	// 5. Security Misconfiguration
	{Keywords: []string{"cors", "header"}, Item: db.SecurityItem{
		Category: "security-misconfig", Description: "Security headers and CORS configuration required",
		Severity: "medium", MitigationStrategy: "Set strict CORS origins; add Content-Security-Policy, X-Frame-Options, X-Content-Type-Options headers",
	}},
	{Keywords: []string{"config", "environment"}, Item: db.SecurityItem{
		Category: "security-misconfig", Description: "Environment-specific configuration hardening required",
		Severity: "medium", MitigationStrategy: "Disable debug mode in production; validate all configuration values at startup; fail fast on missing required config",
	}},

	// 6. XSS (Cross-Site Scripting)
	{Keywords: []string{"input", "render"}, Item: db.SecurityItem{
		Category: "xss", Description: "Cross-site scripting prevention required",
		Severity: "high", MitigationStrategy: "Sanitize all user input; use context-aware output encoding; implement Content-Security-Policy",
	}},

	// 7. Path Traversal / File Access
	{Keywords: []string{"file", "path"}, Item: db.SecurityItem{
		Category: "path-traversal", Description: "File access sandboxing required",
		Severity: "high", MitigationStrategy: "Resolve all paths with realpath(); reject paths outside allowed root; refuse symlinks escaping sandbox",
	}},
	{Keywords: []string{"upload", "file"}, Item: db.SecurityItem{
		Category: "path-traversal", Description: "File upload validation required",
		Severity: "high", MitigationStrategy: "Validate file type via magic bytes (not extension); enforce size limits; store outside web root; generate unique filenames",
	}},

	// 8. Resource Exhaustion
	{Keywords: []string{"timeout", "memory"}, Item: db.SecurityItem{
		Category: "resource-exhaustion", Description: "Resource limits and timeout guards required",
		Severity: "high", MitigationStrategy: "Set per-request timeouts; implement memory watchdog; enforce connection pool limits; add rate limiting",
	}},
	{Keywords: []string{"concurrent", "parallel"}, Item: db.SecurityItem{
		Category: "resource-exhaustion", Description: "Concurrency control required",
		Severity: "medium", MitigationStrategy: "Implement connection pool limits; use bounded worker pools; protect shared state with proper synchronization",
	}},

	// 9. Using Known Vulnerable Components
	{Keywords: []string{"dependency", "package"}, Item: db.SecurityItem{
		Category: "vulnerable-components", Description: "Dependency vulnerability scanning required",
		Severity: "medium", MitigationStrategy: "Run automated dependency audit (npm audit / dotnet list package --vulnerable) in CI; pin major versions; review transitive deps",
	}},

	// 10. Insufficient Logging & Monitoring
	{Keywords: []string{"log", "audit"}, Item: db.SecurityItem{
		Category: "insufficient-logging", Description: "Security event logging required",
		Severity: "medium", MitigationStrategy: "Log all auth events, access control failures, and input validation failures; use structured logging; never log secrets or PII",
	}},
}

// enrichSecurityFromKeywords scans the combined session text (standards + idea + proposal
// narrative) for compound keyword matches and auto-injects corresponding SecurityItems.
// This is the Structural Constraint Linter — deterministic, zero-LLM.
func enrichSecurityFromKeywords(bp *db.Blueprint, session *db.SessionState) {
	// Build the combined corpus for scanning
	var corpus strings.Builder
	for _, std := range session.Standards {
		corpus.WriteString(std)
		corpus.WriteString("\n")
	}
	corpus.WriteString(session.OriginalIdea)
	corpus.WriteString("\n")
	corpus.WriteString(session.RefinedIdea)
	corpus.WriteString("\n")
	if session.DesignProposal != nil {
		corpus.WriteString(session.DesignProposal.Narrative)
		corpus.WriteString("\n")
		for _, mod := range session.DesignProposal.ProposedModules {
			corpus.WriteString(mod.Purpose)
			corpus.WriteString("\n")
			for _, resp := range mod.Responsibilities {
				corpus.WriteString(resp)
				corpus.WriteString("\n")
			}
		}
	}

	corpusLower := strings.ToLower(corpus.String())

	// Build existing categories set to avoid duplicates
	existing := make(map[string]bool)
	for _, item := range bp.SecurityConsiderations {
		existing[item.Category+"|"+item.Description] = true
	}

	injected := 0
	for _, rule := range owaspKeywordRules {
		// Compound match: ALL keywords must appear
		allMatch := true
		for _, keyword := range rule.Keywords {
			if !strings.Contains(corpusLower, keyword) {
				allMatch = false
				break
			}
		}
		if !allMatch {
			continue
		}

		key := rule.Item.Category + "|" + rule.Item.Description
		if existing[key] {
			continue
		}

		bp.SecurityConsiderations = append(bp.SecurityConsiderations, rule.Item)
		existing[key] = true
		injected++
	}

	if injected > 0 {
		slog.Info("constraint_linter: auto-injected security items from keyword scanning",
			"injected", injected,
			"total_rules", len(owaspKeywordRules),
		)
	}
}

// ----- WP-1B: Environment-Specific Constraint Injection -----

// enrichEnvironmentConstraints auto-injects NFRs and security mandates based on the
// session's TargetEnvironment. Each environment has distinct operational constraints
// that should appear in every MADR regardless of what the agent provides.
func enrichEnvironmentConstraints(bp *db.Blueprint, session *db.SessionState) {
	env := strings.ToLower(strings.TrimSpace(session.TargetEnvironment))
	if env == "" {
		return
	}

	var envNFRs []db.NFR
	var envSecurity []db.SecurityItem

	switch {
	case strings.Contains(env, "container") || strings.Contains(env, "docker") || strings.Contains(env, "kubernetes") || strings.Contains(env, "openshift"):
		envNFRs = []db.NFR{
			{Category: "stability", Requirement: "Health check endpoint", Target: "HTTP 200 on /healthz within 100ms", Priority: "must-have"},
			{Category: "stability", Requirement: "Stateless design", Target: "Zero local filesystem state; all persistence via external services", Priority: "must-have"},
			{Category: "performance", Requirement: "Container memory limit", Target: "Memory budget defined; OOM kill prevention with watchdog", Priority: "must-have"},
			{Category: "stability", Requirement: "Graceful container shutdown", Target: "SIGTERM handler drains connections within 10s before exit", Priority: "must-have"},
		}
		envSecurity = []db.SecurityItem{
			{Category: "container-security", Description: "Non-root container execution required", Severity: "high",
				MitigationStrategy: "Run as non-root user; use read-only root filesystem; drop all capabilities except NET_BIND_SERVICE"},
		}

	case strings.Contains(env, "local") || strings.Contains(env, "ide") || strings.Contains(env, "desktop"):
		envNFRs = []db.NFR{
			{Category: "performance", Requirement: "IDE startup latency", Target: "Process ready to accept connections within 500ms", Priority: "must-have"},
			{Category: "stability", Requirement: "Memory ceiling for local process", Target: "Maximum 256MB resident memory; graceful degradation on pressure", Priority: "should-have"},
			{Category: "stability", Requirement: "stdio transport safety", Target: "Zero ANSI/debug output on stdout when used as stdio subprocess", Priority: "must-have"},
		}

	case strings.Contains(env, "cloud") || strings.Contains(env, "aws") || strings.Contains(env, "azure") || strings.Contains(env, "gcp"):
		envNFRs = []db.NFR{
			{Category: "scalability", Requirement: "Horizontal scaling", Target: "Stateless design supports N replicas with zero coordination", Priority: "must-have"},
			{Category: "observability", Requirement: "Distributed tracing", Target: "OpenTelemetry integration with trace context propagation", Priority: "should-have"},
			{Category: "stability", Requirement: "Circuit breaker", Target: "Circuit breaker on all external service calls with fallback strategy", Priority: "should-have"},
			{Category: "security", Requirement: "Secrets rotation", Target: "All secrets from vault/managed service; support zero-downtime rotation", Priority: "must-have"},
		}
		envSecurity = []db.SecurityItem{
			{Category: "cloud-security", Description: "Least-privilege IAM required", Severity: "high",
				MitigationStrategy: "Service identity with minimal permissions; no wildcard policies; rotate credentials automatically"},
		}

	case strings.Contains(env, "edge") || strings.Contains(env, "iot") || strings.Contains(env, "embedded"):
		envNFRs = []db.NFR{
			{Category: "performance", Requirement: "Minimal footprint", Target: "Binary size < 10MB; memory usage < 64MB", Priority: "must-have"},
			{Category: "stability", Requirement: "Offline resilience", Target: "Queue operations when network unavailable; sync on reconnect", Priority: "must-have"},
		}
	}

	// Merge NFRs without duplicates
	existingNFR := make(map[string]bool)
	for _, nfr := range bp.NonFunctionalRequirements {
		existingNFR[nfr.Category+"|"+nfr.Requirement] = true
	}
	for _, nfr := range envNFRs {
		if !existingNFR[nfr.Category+"|"+nfr.Requirement] {
			bp.NonFunctionalRequirements = append(bp.NonFunctionalRequirements, nfr)
		}
	}

	// Merge security items without duplicates
	existingSec := make(map[string]bool)
	for _, item := range bp.SecurityConsiderations {
		existingSec[item.Category+"|"+item.Description] = true
	}
	for _, item := range envSecurity {
		if !existingSec[item.Category+"|"+item.Description] {
			bp.SecurityConsiderations = append(bp.SecurityConsiderations, item)
		}
	}

	if len(envNFRs) > 0 || len(envSecurity) > 0 {
		slog.Info("environment_constraints: auto-injected from target environment",
			"environment", env,
			"nfrs_injected", len(envNFRs),
			"security_injected", len(envSecurity),
		)
	}
}

// ----- WP-1C: Standard-Derived ADR Enrichment -----

// enrichADRsFromStandards scans ingested standards text for content relevant to each
// ADR's topic and extracts supporting citations. This provides the Deterministic MADR
// Population capability — ADR decision drivers and confirmation criteria are derived
// directly from the authoritative standards text stored in BuntDB.
func enrichADRsFromStandards(bp *db.Blueprint, session *db.SessionState) {
	if len(session.Standards) == 0 || len(bp.ADRs) == 0 {
		return
	}

	// Build the combined standards corpus (full-text scan per user directive)
	standardsText := strings.ToLower(strings.Join(session.Standards, "\n---\n"))

	for i := range bp.ADRs {
		adr := &bp.ADRs[i]

		// Extract search terms from the ADR title and decision
		searchTerms := extractSearchTerms(adr.Title + " " + adr.Decision)

		// Skip ADRs that already have rich decision drivers from the agent or previous enrichment
		if len(adr.DecisionDrivers) >= 3 {
			continue
		}

		// Scan standards text for matching paragraphs
		for _, term := range searchTerms {
			if term == "" || len(term) < 3 {
				continue
			}

			termLower := strings.ToLower(term)
			idx := strings.Index(standardsText, termLower)
			if idx == -1 {
				continue
			}

			// Extract a context window around the match (up to 200 chars each side)
			start := idx - 200
			if start < 0 {
				start = 0
			}
			end := idx + len(term) + 200
			if end > len(standardsText) {
				end = len(standardsText)
			}

			// Find sentence boundaries within the window
			snippet := standardsText[start:end]
			sentences := strings.Split(snippet, ".")
			var relevantSentence string
			for _, s := range sentences {
				if strings.Contains(s, termLower) && len(strings.TrimSpace(s)) > 20 {
					relevantSentence = strings.TrimSpace(s)
					break
				}
			}

			if relevantSentence != "" && len(relevantSentence) > 20 {
				driver := fmt.Sprintf("[Standard] %s", relevantSentence)
				// Avoid duplicate drivers
				alreadyExists := false
				for _, d := range adr.DecisionDrivers {
					if d == driver {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					adr.DecisionDrivers = append(adr.DecisionDrivers, driver)
				}
			}

			// Cap at 5 standard-derived drivers per ADR
			if len(adr.DecisionDrivers) >= 5 {
				break
			}
		}

		// Add standard-derived confirmation if missing
		if adr.Confirmation == "" {
			adr.Confirmation = fmt.Sprintf("Verify that %s aligns with ingested architectural standards and passes validation tests", adr.Decision)
		}
	}
}

// extractSearchTerms splits a string into significant search terms,
// filtering out common stop words and short tokens.
func extractSearchTerms(text string) []string {
	stopWords := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "use": true,
		"all": true, "are": true, "was": true, "has": true, "have": true,
		"from": true, "will": true, "can": true, "but": true, "not": true,
		"this": true, "that": true, "into": true, "via": true, "over": true,
	}

	words := strings.Fields(strings.ToLower(text))
	var terms []string
	seen := make(map[string]bool)

	for _, word := range words {
		// Clean punctuation
		clean := strings.Trim(word, ".,;:!?\"'()[]{}")
		if len(clean) < 3 || stopWords[clean] || seen[clean] {
			continue
		}
		seen[clean] = true
		terms = append(terms, clean)
	}
	return terms
}
