package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magictools/internal/dag"
	"mcp-server-magictools/internal/db"
	"mcp-server-magictools/internal/telemetry"
	"mcp-server-magictools/internal/vector"
)

// ---------------------------------------------------------------------------
// Phase 3: Project Manager Pipeline Tools
// ---------------------------------------------------------------------------
// These tools are ONLY available when activePipelineEnabled is true
// (recall + brainstorm + go-refactor all online).

// pipelineGate checks if the active pipeline is enabled.
// Returns an error result if the pipeline is disabled.
func (h *OrchestratorHandler) pipelineGate() (*mcp.CallToolResult, bool) {
	if h.PipelineEnabled == nil || !h.PipelineEnabled.Load() {
		res := &mcp.CallToolResult{}
		res.Content = []mcp.Content{
			&mcp.TextContent{Text: "pipeline_disabled: Active code generation pipeline is offline. Required servers (recall, brainstorm, go-refactor) are not all available."},
		}
		return res, false
	}
	return nil, true
}

// RegisterPipelineTools registers the PM tools on the MCP server.
func (h *OrchestratorHandler) RegisterPipelineTools(s *mcp.Server) {
	h.addTool(s, &mcp.Tool{Name: "compose_pipeline"}, h.handleComposePipeline)
	h.addTool(s, &mcp.Tool{Name: "validate_pipeline_step"}, h.handleValidatePipelineStep)
	h.addTool(s, &mcp.Tool{Name: "cross_server_quality_gate"}, h.handleQualityGate)
	h.addTool(s, &mcp.Tool{Name: "generate_audit_report"}, h.handleGenerateAuditReport)

	slog.Info("pipeline tools registered", "component", "pipeline", "count", 4)
}

// ---------------------------------------------------------------------------
// compose_pipeline handler
// ---------------------------------------------------------------------------

func (h *OrchestratorHandler) handleComposePipeline(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if res, ok := h.pipelineGate(); !ok {
		return res, nil
	}

	var args struct {
		ProjectPath string   `json:"project_path"`
		Intent      string   `json:"intent"`
		TargetRoles []string `json:"target_roles"`
	}
	_ = json.Unmarshal(req.Params.Arguments, &args)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Pipeline Plan for: %s\n\n", args.ProjectPath))
	sb.WriteString(fmt.Sprintf("**Intent**: %s\n\n", args.Intent))

	// Query recall for project history (used for context enrichment, NOT BM25 scoring).
	var projectHistory string
	if h.RecallClient != nil && h.RecallClient.RecallEnabled() {
		projectHistory = h.RecallClient.ListSessionsByFilter(ctx, args.ProjectPath, "", "", 5)
	}

	// Query recall for relevant standards (intent-derived for precision).
	var standards string
	if h.RecallClient != nil && h.RecallClient.RecallEnabled() {
		standardsQuery := args.Intent + " best practices code quality"
		standards = h.RecallClient.SearchStandards(ctx, standardsQuery, "", "", 10)
	}

	// Build the recommended pipeline stages.
	sb.WriteString("## Recommended Pipeline Stages\n\n")

	// 🛡️ SERVER-FILTERED TOOL REGISTRY: Only brainstorm and go-refactor tools enter the DAG.
	// Query each server directly to avoid fetching all 135+ tools and discarding most.
	brainstormRecords, _ := h.Store.SearchTools("", "", "brainstorm", 0.0)
	goRefactorRecords, _ := h.Store.SearchTools("", "", "go-refactor", 0.0)
	pipelineRecords := make([]*db.ToolRecord, 0, len(brainstormRecords)+len(goRefactorRecords))
	for _, r := range brainstormRecords {
		if r != nil {
			pipelineRecords = append(pipelineRecords, r)
		}
	}
	for _, r := range goRefactorRecords {
		if r != nil {
			pipelineRecords = append(pipelineRecords, r)
		}
	}

	intent := strings.ToLower(args.Intent)
	stages, gatekeeperWarnings := h.executeSwarmBidding(ctx, intent, args.TargetRoles, pipelineRecords)

	for i, stage := range stages {
		roleTag := ""
		if stage.Role != "" {
			roleTag = fmt.Sprintf(" [%s]", stage.Role)
		}
		phaseTag := ""
		if stage.Phase > 0 {
			phaseTag = fmt.Sprintf(" (Phase %d)", stage.Phase)
		}
		sb.WriteString(fmt.Sprintf("%d. **%s**%s%s — %s\n", i+1, stage.ToolName, roleTag, phaseTag, stage.Purpose))
	}

	// 🛡️ SEMANTIC GATEKEEPER: Validate DAG ordering for structural anti-patterns.
	if strictWarnings := validateDAGSemantics(stages); len(strictWarnings) > 0 {
		gatekeeperWarnings = append(gatekeeperWarnings, strictWarnings...)
	}

	if len(gatekeeperWarnings) > 0 {
		sb.WriteString("\n## ⚠️ Semantic Gatekeeper Warnings\n\n")
		for _, w := range gatekeeperWarnings {
			sb.WriteString("- " + w + "\n")
		}
		sb.WriteString("\n")
	}

	if projectHistory != "" {
		sb.WriteString("\n## Project History (from Recall)\n")
		sb.WriteString(projectHistory + "\n")
	}

	if standards != "" {
		sb.WriteString("\n## Applicable Standards (from Recall)\n")
		sb.WriteString(standards + "\n")
	}

	sb.WriteString("\n## Execution Notes\n")
	sb.WriteString("- Execute stages sequentially — each stage depends on the previous.\n")
	sb.WriteString("- Phase ordering: BOOTSTRAP → ANALYSIS → ADVERSARIAL → PROPOSAL → CRITIQUE → SYNTHESIS → PLANNER → MUTATOR → VALIDATION → TERMINAL.\n")
	sb.WriteString("- Only MUTATOR-role tools may write to the filesystem.\n")

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
	}, nil
}

// PipelineStep represents a single recommended stage natively mapping tight Go bounds.
type PipelineStep struct {
	ToolName       string         `json:"name"`
	Role           string         `json:"role"`
	Phase          int            `json:"phase"`
	Purpose        string         `json:"purpose"`
	Args           map[string]any `json:"args,omitzero"`
	InputContract  string         `json:"input_contract,omitempty"`
	OutputContract string         `json:"output_contract,omitempty"`
}

// scoredTool pairs a tool record with its computed pipeline score.
type scoredTool struct {
	record     *db.ToolRecord
	finalScore float64
}

// executeSwarmBidding uses intent-weighted tri-factor scoring with phase sequencing
// to compose a pipeline DAG from go-refactor and brainstorm tools only.
//
// Scoring formula: S_final = (S_engine × wEngine) + (S_synergy × wSynergy) + (R_role × wRole)
// where S_engine = (α × cosine) + ((1-α) × bm25) for hybrid mode, or pure bm25 for offline mode.
func (h *OrchestratorHandler) executeSwarmBidding(ctx context.Context, intent string, targetRoles []string, pipelineTools []*db.ToolRecord) ([]PipelineStep, []string) {
	var stages []PipelineStep

	// 1. Classify intent for role boosting
	intentType := classifyIntent(intent)

	// 1b. GHOST INDEX PROBE: Extract mathematically validated historical DAG arrays locally
	now := time.Now().Unix()
	ghostMap := make(map[string]int64) // URN → creation timestamp for decay
	if h.Store != nil && h.Store.Index != nil {
		if ghostResults, err := h.Store.Index.SearchSyntheticIntents(intent); err == nil {
			for _, g := range ghostResults {
				ghostMap[g.URN] = g.Timestamp
			}
		}
	}

	// 🛡️ TELEMETRY FIX: Ensure we track the global search request for metric visibility.
	telemetry.SearchMetrics.TotalSearches.Add(1)

	// 2. Initialize Core Engine (S_Cosine vs S_Bleve Split Path)
	scores := make(map[string]float64)
	e := vector.GetEngine()
	engineType := "BM25"

	// 🛡️ FIX #5 — HYBRID RRF FUSION: Always compute BM25 as the baseline.
	bm25Scores := getOfflineBM25Scores(intent, pipelineTools)

	if e != nil && e.VectorEnabled() {
		// ONLINE MODE: Direct score fusion with actual cosine similarity
		matchedResults, err := e.SearchWithScores(ctx, intent, 20)
		if err == nil {
			engineType = "Hybrid-Fusion"

			// Build cosine score map from vector results
			vectorScores := make(map[string]float64)
			for _, r := range matchedResults {
				vectorScores[r.Key] = r.Score
			}

			// Direct score fusion: α*cosine + (1-α)*bm25
			// Both engines produce [0,1] normalized scores, so direct combination
			// preserves full discriminative spread unlike rank-based RRF.
			alpha := 0.6
			if h.Config != nil && h.Config.ScoreFusionAlpha > 0 {
				alpha = h.Config.ScoreFusionAlpha
			}

			allURNs := make(map[string]bool)
			for urn := range bm25Scores {
				allURNs[urn] = true
			}
			for urn := range vectorScores {
				allURNs[urn] = true
			}

			var vectorDominant, lexicalDominant int64
			for urn := range allURNs {
				cosine, hasVector := vectorScores[urn]
				bm25, hasBM25 := bm25Scores[urn]

				switch {
				case hasVector && hasBM25:
					scores[urn] = alpha*cosine + (1-alpha)*bm25
				case hasVector:
					scores[urn] = cosine * alpha // Natural penalty: no BM25 confirmation
				case hasBM25:
					scores[urn] = bm25 * (1 - alpha) // Natural penalty: no vector confirmation
				}

				// Telemetry: track which engine contributed more
				if cosine > bm25 {
					vectorDominant++
				} else if bm25 > cosine {
					lexicalDominant++
				}
			}
			telemetry.SearchMetrics.VectorWins.Add(vectorDominant)
			telemetry.SearchMetrics.LexicalWins.Add(lexicalDominant)
		} else {
			slog.Warn("compose_pipeline: vector search failed, falling back to BM25 natively", "error", err)
			scores = bm25Scores
		}
	} else {
		// OFFLINE MODE: Pure BM25 (already normalized to [0,1])
		scores = bm25Scores
	}

	// Dynamic target roles resolution map natively ensuring strict constraints if explicitly defined
	roleMap := make(map[string]bool)
	for _, tr := range targetRoles {
		roleMap[strings.ToUpper(tr)] = true
	}

	// 3. Intent-Weighted Tri-Factor Scoring
	var candidates []scoredTool

	for _, t := range pipelineTools {
		if t == nil {
			continue
		}

		// Skip diagnostic tools — they are out-of-band
		if t.Role == "DIAGNOSTIC" {
			continue
		}

		// Check dynamic target role exclusivity logic structurally mapped correctly natively
		if len(roleMap) > 0 {
			if !roleMap[t.Role] {
				continue
			}
		}

		// S_engine: Vector cosine or legacy BM25 (normalized 0-1 natively map scaling)
		sEngine := scores[t.URN]

		// S_synergy: Ghost index confidence with exponential decay (~72h half-life)
		sSynergy := 0.0
		if ts, ok := ghostMap[t.URN]; ok && ts > 0 {
			ageHours := float64(now-ts) / 3600.0
			const halfLifeHours = 72.0
			sSynergy = math.Exp(-0.693 * ageHours / halfLifeHours)
		} else if _, ok := ghostMap[t.URN]; ok {
			// Ghost entry exists but has no timestamp (pre-decay era) — use flat score
			sSynergy = 1.0
		}

		// R_role: Intent-Role alignment boost (+1.0 if aligned, 0.0 otherwise)
		rRole := computeRoleBoost(t.Role, intentType)

		// RRF Biases securely hooked dynamically explicitly via local Config
		biasVector := 0.5
		biasSynergy := 0.2
		biasRole := 0.3
		if h.Config != nil {
			biasVector = h.Config.SynthesisBiasVector
			biasSynergy = h.Config.SynthesisBiasSynergy
			biasRole = h.Config.SynthesisBiasRole
		}

		// Tri-factor scoring: engine × wEngine + synergy × wSynergy + role × wRole
		sFinal := (sEngine * biasVector) + (sSynergy * biasSynergy) + (rRole * biasRole)

		candidates = append(candidates, scoredTool{record: t, finalScore: sFinal})
	}

	// 4. Dynamic Thresholding — adaptive stddev-based threshold
	var totalScore float64
	var count float64
	for _, c := range candidates {
		if c.finalScore > 0 {
			totalScore += c.finalScore
			count++
		}
	}
	threshold := 0.1 // Safety floor
	if count > 0 {
		var activeScores []float64
		for _, c := range candidates {
			if c.finalScore > 0 {
				activeScores = append(activeScores, c.finalScore)
			}
		}

		sort.Float64s(activeScores)
		n := len(activeScores)
		var median float64
		if n > 0 {
			if n%2 == 0 {
				median = (activeScores[n/2-1] + activeScores[n/2]) / 2.0
			} else {
				median = activeScores[n/2]
			}
		}

		var absDevs []float64
		for _, s := range activeScores {
			absDevs = append(absDevs, math.Abs(s-median))
		}
		sort.Float64s(absDevs)

		var mad float64
		if n > 0 {
			if n%2 == 0 {
				mad = (absDevs[n/2-1] + absDevs[n/2]) / 2.0
			} else {
				mad = absDevs[n/2]
			}
		}

		// Ensure MAD has a minimal threshold dynamically mitigating completely homogenised loops
		if mad < 0.05 {
			mad = 0.05
		}

		// 🛡️ PIPELINE OPTIMIZATION: Median Absolute Deviation (MAD) Thresholding
		dynamicThreshold := median + mad
		if dynamicThreshold > threshold {
			threshold = dynamicThreshold
		}
	}

	var qualified []scoredTool
	hasMutator := false
	for _, c := range candidates {
		if c.finalScore >= threshold {
			qualified = append(qualified, c)
			if c.record.Role == "MUTATOR" {
				hasMutator = true
			}
		}
	}

	// 🛡️ OPTIMIZATION 1: Adjacency Thresholds (Synergy Injection)
	// If a RoleMutator triggers, dynamically inject critique tools from Brainstorm to break Phase isolation
	// and eliminate mathematical Intent starvation.
	if hasMutator {
		for _, c := range candidates {
			if c.record.Server == "brainstorm" && c.record.Role == "CRITIC" {
				exists := false
				for _, q := range qualified {
					if q.record.URN == c.record.URN {
						exists = true
						break
					}
				}
				if !exists {
					// Proportional scale dynamically bypassing arbitrary additive constants natively
					c.finalScore *= 1.35 // Inject proportional synergy to safely bypass intent threshold
					if c.finalScore >= threshold {
						qualified = append(qualified, c)
					}
				}
			}
		}
	}

	// 5. Phase Sequencing Grammar: Sort by Phase first, then by score within each phase
	sort.SliceStable(qualified, func(i, j int) bool {
		if qualified[i].record.Phase != qualified[j].record.Phase {
			return qualified[i].record.Phase < qualified[j].record.Phase
		}
		return qualified[i].finalScore > qualified[j].finalScore
	})

	// 🛡️ EDGE WEIGHT ADJACENCY BOOST: Nudge adjacent stage scores using historical
	// transition success data from the synergy tracking system. Applied before phase-role
	// pruning so historically proven tools can survive the per-group cap.
	if h.Store != nil {
		for i := 0; i < len(qualified)-1; i++ {
			edgeScore := dag.GetEdgeScore(h.Store, qualified[i].record.URN, qualified[i+1].record.URN)
			if edgeScore > 0.5 {
				qualified[i+1].finalScore += edgeScore * 0.1 // Gentle 10% boost for proven transitions
			}
		}
	}

	// 🛡️ PHASE-ROLE CLUSTER PRUNING: Strict singleton mapping. Limit to exactly 1 tool per (phase, role) group natively.
	// This prevents pathological clustering and natively shrinks pipeline steps.
	qualified = prunePhaseRoleClusters(qualified, 1)

	// 6. Compose final stages with phase-enriched metadata
	for _, q := range qualified {
		purpose := fmt.Sprintf("Score: %.2f (%s: %.2f, Role: %s, Phase: %d)",
			q.finalScore, engineType, scores[q.record.URN], q.record.Role, q.record.Phase)
		stages = append(stages, PipelineStep{
			ToolName:       q.record.URN,
			Role:           q.record.Role,
			Phase:          q.record.Phase,
			Purpose:        purpose,
			Args:           nil,
			InputContract:  q.record.InputContract,
			OutputContract: q.record.OutputContract,
		})
	}

	// 🛡️ Socratic Option B Anchor Resolver (Dynamic DAG Generation)
	stages = resolveDynamicDAG(stages, pipelineTools)

	// 🛡️ DFS Topological Mapping: Kahn's Gatekeeper Algorithm natively replacing SliceStable flaws.
	stages, warnings := topologicalSort(stages, pipelineTools)

	stages = append(stages, PipelineStep{
		ToolName: "magictools:generate_audit_report",
		Role:     "SYNTHESIZER",
		Phase:    8,
		Purpose:  "Generate git diff and formal markdown compliance report",
	})

	// 🛡️ Mutual Exclusivity Enclaves: Prune colliding analyzers internally
	stages = enforceExclusivityEnclaves(stages)

	// 🛡️ Socratic Markdown Injector
	// Determine natively if the Aporia Engine or Suggest Fixes are active mapped tools
	hasSocraticEngine := false
	for _, s := range stages {
		if strings.Contains(s.ToolName, "aporia_engine") || strings.Contains(s.ToolName, "suggest_fixes") {
			hasSocraticEngine = true
			break
		}
	}

	if hasSocraticEngine {
		// Organically build the Markdown list logic traces so the LLM evaluates the JSON telemetry
		stages = append(stages, PipelineStep{
			ToolName: "magictools:cross_server_quality_gate",
			Role:     "GATEKEEPER",
			Phase:    7,
			Purpose:  "Verify mathematical standards, Brainstorm approvals, and prior analyses before allowing file system mutation natively.",
		})
		stages = append(stages, PipelineStep{
			ToolName: "go-refactor:apply_vetted_edit",
			Role:     "MUTATOR",
			Phase:    8,
			Purpose:  "CONDITIONAL GATEKEEPER: Execute MUTATOR natively ONLY IF previous Socratic node emitted ADOPT or SUCCESS payload logic.",
		})
		stages = append(stages, PipelineStep{
			ToolName: "go-refactor:go_test_validation",
			Role:     "VALIDATOR",
			Phase:    9,
			Purpose:  "CONDITIONAL GATEKEEPER: Run automatically verifying structural test constraints post-mutation natively.",
		})
		stages = append(stages, PipelineStep{
			ToolName: "magictools:validate_pipeline_step",
			Role:     "VALIDATOR",
			Phase:    10,
			Purpose:  "Scan execution outputs natively for fatal panics, semantic degradation, or structural test failures.",
		})
	}

	return stages, warnings
}

// resolveDynamicDAG continuously walks the explicit Requires/Triggers constraints provided by local MCP Sub-Servers.
// Recursively extracts properties ensuring no dependency limit violates pure topology limits physically tracking graph logic dynamically.
func resolveDynamicDAG(stages []PipelineStep, pipelineTools []*db.ToolRecord) []PipelineStep {
	registry := make(map[string]*db.ToolRecord)
	for _, t := range pipelineTools {
		registry[t.URN] = t
	}

	for {
		addedNewNode := false

		selected := make(map[string]bool)
		for _, s := range stages {
			selected[s.ToolName] = true
		}

		for _, s := range stages {
			if rec, exists := registry[s.ToolName]; exists {
				for _, req := range rec.Requires {
					if !selected[req] {
						if target, ok := registry[req]; ok {
							stages = append(stages, PipelineStep{
								ToolName:       target.URN,
								Role:           target.Role,
								Phase:          target.Phase,
								Purpose:        fmt.Sprintf("Dynamic DAG Requirement: Anchoring execution mapped natively off %s bounds.", s.ToolName),
								InputContract:  target.InputContract,
								OutputContract: target.OutputContract,
							})
							selected[req] = true
							addedNewNode = true
						}
					}
				}
				for _, trig := range rec.Triggers {
					if !selected[trig] {
						if target, ok := registry[trig]; ok {
							stages = append(stages, PipelineStep{
								ToolName:       target.URN,
								Role:           target.Role,
								Phase:          target.Phase,
								Purpose:        fmt.Sprintf("Dynamic DAG Trigger: Fired organically executing %s dependency limits.", s.ToolName),
								InputContract:  target.InputContract,
								OutputContract: target.OutputContract,
							})
							selected[trig] = true
							addedNewNode = true
						}
					}
				}
			}
		}

		if !addedNewNode {
			break
		}
	}
	return stages
}

// topologicalSort implements strict DFS Topological bounding organically tracing Acyclic structures ensuring formal Requires constraints are mapped mathematically safely bypassing SliceStable index faults natively.
func topologicalSort(stages []PipelineStep, pipelineTools []*db.ToolRecord) ([]PipelineStep, []string) {
	registry := make(map[string]*db.ToolRecord)
	for _, t := range pipelineTools {
		registry[t.URN] = t
	}

	var warnings []string

	// Fallback Phase precedence sort structurally natively avoiding timeline jitter iteratively.
	sort.SliceStable(stages, func(i, j int) bool {
		return stages[i].Phase < stages[j].Phase
	})

	adj := make(map[string][]string)
	nodes := make(map[string]PipelineStep)
	var orderedNames []string

	for _, s := range stages {
		nodes[s.ToolName] = s
		orderedNames = append(orderedNames, s.ToolName)
	}

	for _, s := range stages {
		if rec, exists := registry[s.ToolName]; exists {
			for _, requiredURN := range rec.Requires {
				if _, ok := nodes[requiredURN]; ok {
					adj[requiredURN] = append(adj[requiredURN], s.ToolName) // requiredURN -> s.ToolName
				}
			}
			for _, triggerURN := range rec.Triggers {
				if _, ok := nodes[triggerURN]; ok {
					adj[s.ToolName] = append(adj[s.ToolName], triggerURN) // s.ToolName -> triggerURN
				}
			}
		}
	}

	state := make(map[string]int) // 0=Unvisited, 1=Visiting, 2=Visited
	var resultNames []string

	var dfs func(urn string) bool
	dfs = func(urn string) bool {
		if state[urn] == 1 {
			return true // Cycle detected
		}
		if state[urn] == 2 {
			return false
		}

		state[urn] = 1
		for _, neighbor := range adj[urn] {
			if dfs(neighbor) {
				warnings = append(warnings, fmt.Sprintf("⚠️ Semantic Gatekeeper Warning: Circular Dependency Detected connecting `%s` natively.", neighbor))
			}
		}
		state[urn] = 2
		resultNames = append([]string{urn}, resultNames...)
		return false
	}

	// Traverse roots explicitly preserving internal chronological boundaries structurally cleanly.
	inDegree := make(map[string]int)
	for _, neighbors := range adj {
		for _, n := range neighbors {
			inDegree[n]++
		}
	}

	for i := len(orderedNames) - 1; i >= 0; i-- {
		urn := orderedNames[i]
		if inDegree[urn] == 0 && state[urn] == 0 {
			dfs(urn)
		}
	}

	// Sweep disjoint components actively.
	for i := len(orderedNames) - 1; i >= 0; i-- {
		urn := orderedNames[i]
		if state[urn] == 0 {
			dfs(urn)
		}
	}

	var finalStages []PipelineStep
	for _, urn := range resultNames {
		finalStages = append(finalStages, nodes[urn])
	}

	return finalStages, warnings
}

func getOfflineBM25Scores(intent string, pipelineTools []*db.ToolRecord) map[string]float64 {
	pruned := pruneIntent(intent)
	scorer := dag.NewBM25Scorer(pipelineTools)
	return scorer.Score(pruned)
}

// pruneIntent mathematically strips grammatical buzzwords strictly guaranteeing dense token scoring matching natively structurally.
func pruneIntent(text string) string {
	stopWords := []string{"i", "need", "to", "the", "a", "an", "and", "or", "in", "on", "for", "with", "this", "that", "it", "of", "by", "as", "is", "are"}
	words := strings.Fields(strings.ToLower(text))
	var result []string

	stopMap := make(map[string]bool)
	for _, w := range stopWords {
		stopMap[w] = true
	}

	for _, w := range words {
		if !stopMap[w] {
			result = append(result, w)
		}
	}
	return strings.Join(result, " ")
}

// enforceExclusivityEnclaves structurally removes dynamic DAG looping nodes gracefully filtering array density.
func enforceExclusivityEnclaves(stages []PipelineStep) []PipelineStep {
	var finalStages []PipelineStep
	seen := make(map[string]bool)

	for _, s := range stages {
		// Prevent duplicates natively guaranteeing linear purity locally
		if seen[s.ToolName] {
			continue
		}
		seen[s.ToolName] = true
		finalStages = append(finalStages, s)
	}
	return finalStages
}

// prunePhaseRoleClusters caps the number of tools per (phase, role) pair.
// The input must already be sorted by phase ascending, score descending.
// This prevents pathological clustering (e.g., 7 CRITICs in Phase 4)
// while keeping the highest-scoring tools from each group.
func prunePhaseRoleClusters(tools []scoredTool, maxPerGroup int) []scoredTool {
	type phaseRole struct {
		phase int
		role  string
	}
	counts := make(map[phaseRole]int)
	var result []scoredTool

	for _, t := range tools {
		key := phaseRole{phase: t.record.Phase, role: t.record.Role}
		if counts[key] >= maxPerGroup {
			continue
		}
		counts[key]++
		result = append(result, t)
	}
	return result
}

// classifyIntent determines the dominant intent category from the user's request.
// Returns one of: "audit", "refactor", "plan"
func classifyIntent(intent string) string {
	intent = strings.ToLower(intent)

	auditSignals := []string{"audit", "analyze", "review", "inspect", "assess", "evaluate", "check", "scan", "trace"}
	refactorSignals := []string{"refactor", "fix", "modernize", "optimize", "clean", "prune", "migrate", "upgrade"}
	planSignals := []string{"plan", "design", "architect", "propose", "strategy", "blueprint", "feature"}

	auditScore, refactorScore, planScore := 0, 0, 0
	for _, s := range auditSignals {
		if strings.Contains(intent, s) {
			auditScore++
		}
	}
	for _, s := range refactorSignals {
		if strings.Contains(intent, s) {
			refactorScore++
		}
	}
	for _, s := range planSignals {
		if strings.Contains(intent, s) {
			planScore++
		}
	}

	if refactorScore >= auditScore && refactorScore >= planScore {
		return "refactor"
	}
	if planScore >= auditScore {
		return "plan"
	}
	return "audit"
}

// computeRoleBoost returns a graduated boost based on Role-Intent alignment.
// Uses a 4-level gradient instead of binary 0/1 to prevent cliff-edge scoring.
func computeRoleBoost(role, intentType string) float64 {
	switch intentType {
	case "audit":
		switch role {
		case "ANALYZER":
			return 1.0
		case "CRITIC":
			return 0.7
		case "SYNTHESIZER":
			return 0.4
		case "MUTATOR":
			return 0.2
		}
	case "refactor":
		switch role {
		case "MUTATOR":
			return 1.0
		case "ANALYZER":
			return 0.75
		case "CRITIC":
			return 0.5
		case "SYNTHESIZER":
			return 0.25
		}
	case "plan":
		switch role {
		case "CRITIC", "SYNTHESIZER", "PLANNER":
			return 1.0
		case "ANALYZER":
			return 0.5
		case "MUTATOR":
			return 0.25
		}
	}
	return 0.0
}

// validateDAGSemantics checks the composed pipeline DAG for structural
// anti-patterns that would produce invalid execution orderings natively.
// Returns a list of human-readable warnings (empty = clean DAG).
func validateDAGSemantics(stages []PipelineStep) []string {
	var warnings []string

	firstAnalyzer, firstCritic, firstMutator, firstSynthesizer, firstPlanner := -1, -1, -1, -1, -1

	for i, s := range stages {
		switch s.Role {
		case "ANALYZER":
			if firstAnalyzer == -1 {
				firstAnalyzer = i
			}
		case "CRITIC":
			if firstCritic == -1 {
				firstCritic = i
			}
		case "MUTATOR":
			if firstMutator == -1 {
				firstMutator = i
			}
		case "SYNTHESIZER":
			if firstSynthesizer == -1 {
				firstSynthesizer = i
			}
		case "PLANNER":
			if firstPlanner == -1 {
				firstPlanner = i
			}
		}
	}

	// 🛡️ Phase Grammar Sequence Enclaves
	if firstMutator >= 0 && firstAnalyzer >= 0 && firstMutator < firstAnalyzer {
		warnings = append(warnings, "Grammar Violation: MUTATOR tool appears before ANALYZER tools — mutations must follow analysis.")
	}
	if firstMutator >= 0 && firstPlanner >= 0 && firstMutator < firstPlanner {
		warnings = append(warnings, "Grammar Violation: MUTATOR tool appears before PLANNER tools — mutations must follow formal planning.")
	}
	if firstPlanner >= 0 && firstSynthesizer >= 0 && firstPlanner < firstSynthesizer {
		warnings = append(warnings, "Grammar Violation: PLANNER tool appears before SYNTHESIZER tools — planning requires synthesized diagnostics.")
	}
	if firstSynthesizer >= 0 && firstCritic >= 0 && firstSynthesizer < firstCritic {
		warnings = append(warnings, "Grammar Violation: SYNTHESIZER tool appears before CRITIC tools — synthesis requires adversarial verdicts.")
	}

	// 🛡️ DATA-CONTRACT CHAIN VALIDATION: Check consecutive stage contract compatibility.
	for i := 0; i < len(stages)-1; i++ {
		if stages[i].OutputContract != "" && stages[i+1].InputContract != "" {
			if stages[i].OutputContract != stages[i+1].InputContract {
				warnings = append(warnings, fmt.Sprintf(
					"Data Contract Mismatch: Stage %d (%s) outputs [%s] but Stage %d (%s) expects [%s].",
					i+1, stages[i].ToolName, stages[i].OutputContract,
					i+2, stages[i+1].ToolName, stages[i+1].InputContract))
			}
		}
	}

	// 🛡️ DEAD-END DETECTION: Warn if a stage's output is never consumed by any subsequent stage.
	for i := 0; i < len(stages)-1; i++ {
		if stages[i].OutputContract == "" {
			continue
		}
		consumed := false
		for j := i + 1; j < len(stages); j++ {
			if stages[j].InputContract == stages[i].OutputContract {
				consumed = true
				break
			}
		}
		if !consumed {
			warnings = append(warnings, fmt.Sprintf(
				"Data Contract Gap: Stage %d (%s) outputs [%s] but no subsequent stage consumes it.",
				i+1, stages[i].ToolName, stages[i].OutputContract))
		}
	}

	// 🛡️ REDUNDANCY GOVERNOR: Warn if 3+ consecutive stages share the same Role.
	// This detects pathological DAGs (e.g., three analyzers without an intervening mutator)
	// that waste agent execution cycles without advancing the pipeline.
	consecutiveCount := 1
	for i := 1; i < len(stages); i++ {
		if stages[i].Role != "" && stages[i].Role == stages[i-1].Role {
			consecutiveCount++
			if consecutiveCount >= 3 {
				warnings = append(warnings, fmt.Sprintf(
					"Redundancy Warning: %d consecutive %s-role tools (%s..%s) — consider interleaving a different role.",
					consecutiveCount, stages[i].Role, stages[i-consecutiveCount+1].ToolName, stages[i].ToolName))
			}
		} else {
			consecutiveCount = 1
		}
	}

	return warnings
}

// ---------------------------------------------------------------------------
// validate_pipeline_step handler
// ---------------------------------------------------------------------------

func (h *OrchestratorHandler) handleValidatePipelineStep(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if res, ok := h.pipelineGate(); !ok {
		return res, nil
	}

	var args struct {
		StepName    string `json:"step_name"`
		StepOutput  string `json:"step_output"`
		ProjectPath string `json:"project_path"`
	}
	_ = json.Unmarshal(req.Params.Arguments, &args)

	// Query relevant standards for this specific tool.
	var standards string
	if h.RecallClient != nil && h.RecallClient.RecallEnabled() {
		standards = h.RecallClient.SearchStandards(ctx, fmt.Sprintf("%s validation quality criteria", args.StepName), "", "", 5)
	}

	// Validate step output.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Step Validation: %s\n\n", args.StepName))

	// Check for error indicators in output.
	verdict := "PASS"
	var issues []string

	outputLower := strings.ToLower(args.StepOutput)
	if strings.Contains(outputLower, "error") || strings.Contains(outputLower, "fatal") {
		issues = append(issues, "Output contains error indicators")
		verdict = "NEEDS_REVIEW"
	}
	if strings.Contains(outputLower, "panic") || strings.Contains(outputLower, "crash") {
		issues = append(issues, "Output contains critical failure indicators")
		verdict = "FAIL"
	}
	if len(args.StepOutput) < 50 {
		issues = append(issues, "Output is suspiciously short — may indicate tool failure")
		verdict = "NEEDS_REVIEW"
	}

	sb.WriteString(fmt.Sprintf("**Verdict**: %s\n\n", verdict))

	if len(issues) > 0 {
		sb.WriteString("## Issues Detected\n")
		for _, issue := range issues {
			sb.WriteString(fmt.Sprintf("- %s\n", issue))
		}
		sb.WriteString("\n")
	}

	if standards != "" {
		sb.WriteString("## Applicable Standards\n")
		sb.WriteString(standards + "\n")
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
	}, nil
}

// ---------------------------------------------------------------------------
// cross_server_quality_gate handler
// ---------------------------------------------------------------------------

func (h *OrchestratorHandler) handleQualityGate(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if res, ok := h.pipelineGate(); !ok {
		return res, nil
	}

	var args struct {
		ProjectPath string `json:"project_path"`
		PlanHash    string `json:"plan_hash"`
	}
	_ = json.Unmarshal(req.Params.Arguments, &args)

	var sb strings.Builder
	sb.WriteString("# Cross-Server Quality Gate\n\n")
	sb.WriteString(fmt.Sprintf("**Project**: %s\n", args.ProjectPath))
	sb.WriteString(fmt.Sprintf("**Plan Hash**: %s\n\n", args.PlanHash))

	checks := make(map[string]string)

	// Check 1: Recall — verify standards compliance.
	if h.RecallClient != nil && h.RecallClient.RecallEnabled() {
		cats := h.RecallClient.ListStandardsCategories(ctx, "")
		if cats != "" {
			checks["recall_standards"] = "✅ Standards database accessible"
		} else {
			checks["recall_standards"] = "⚠️ Standards database empty or unreachable"
		}

		// Check for approval in sessions.
		approved, err := h.RecallClient.CheckApprovalExists(ctx, args.ProjectPath, args.PlanHash)
		if err != nil {
			checks["brainstorm_approval"] = fmt.Sprintf("⚠️ Approval check failed: %v", err)
		} else if approved {
			checks["brainstorm_approval"] = "✅ Plan approved by brainstorm vetting"
		} else {
			checks["brainstorm_approval"] = "❌ No approval found for this plan_hash"
		}

		// Check for go-refactor analysis completion.
		refactorHistory := h.RecallClient.ListSessionsByFilter(ctx, args.ProjectPath, "go-refactor", "completed", 5)
		if refactorHistory != "" {
			checks["gorefactor_analysis"] = "✅ Go-refactor analysis completed"
		} else {
			checks["gorefactor_analysis"] = "⚠️ No completed go-refactor analysis found"
		}
	} else {
		checks["recall_connection"] = "❌ Recall client not available"
	}

	// Determine overall gate verdict.
	sb.WriteString("## Quality Checks\n\n")
	gatePass := true
	for check, status := range checks {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", check, status))
		if strings.HasPrefix(status, "❌") {
			gatePass = false
		}
	}

	sb.WriteString("\n## Gate Verdict\n\n")
	if gatePass {
		sb.WriteString("**✅ GATE PASSED** — All quality checks satisfied. Safe to proceed with `apply_vetted_edit`.\n")
	} else {
		sb.WriteString("**❌ GATE FAILED** — One or more quality checks failed. Do NOT proceed with filesystem writes.\n")
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
	}, nil
}
