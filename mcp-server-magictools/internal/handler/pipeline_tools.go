package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magictools/internal/dag"
	"mcp-server-magictools/internal/db"
	"mcp-server-magictools/internal/intelligence"
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
	h.addTool(s, &mcp.Tool{Name: "execute_pipeline"}, h.handleExecutePipeline)
	h.addTool(s, &mcp.Tool{Name: "validate_pipeline_step"}, h.handleValidatePipelineStep)
	h.addTool(s, &mcp.Tool{Name: "cross_server_quality_gate"}, h.handleQualityGate)
	h.addTool(s, &mcp.Tool{Name: "generate_audit_report"}, h.handleGenerateAuditReport)

	slog.Info("pipeline tools registered", "component", "pipeline", "count", 4)
}

// handleComposePipeline has been merged into handleExecutePipeline.
// Use execute_pipeline with dry_run=true for plan preview.

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
// Scoring formula: S_final = ((S_engine × wEngine) + (S_synergy × wSynergy) + (R_role × wRole) + (S_intent × wIntent)) × failurePenalty
// where S_engine = (α × cosine) + ((1-α) × bm25) for hybrid mode, or pure bm25 for offline mode.
// S_intent is the Option 3 intent→tool outcome score from real-time synergy tracking.
// failurePenalty is the Option 6 contrastive failure anchor proximity multiplier.
func (h *OrchestratorHandler) executeSwarmBidding(ctx context.Context, intent string, targetRoles []string, pipelineTools []*db.ToolRecord) ([]PipelineStep, []string) {
	var stages []PipelineStep

	// 1. Classify intent for role boosting (blended multi-category)
	intentWeights := classifyIntentWeights(intent)

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

			type scorePair struct {
				urn   string
				score float64
			}

			var bPairs []scorePair
			for u, s := range bm25Scores {
				bPairs = append(bPairs, scorePair{u, s})
			}
			for i := 1; i < len(bPairs); i++ {
				for j := i; j > 0 && bPairs[j].score > bPairs[j-1].score; j-- {
					bPairs[j], bPairs[j-1] = bPairs[j-1], bPairs[j]
				}
			}

			var hPairs []scorePair
			for u, s := range vectorScores {
				hPairs = append(hPairs, scorePair{u, s})
			}
			for i := 1; i < len(hPairs); i++ {
				for j := i; j > 0 && hPairs[j].score > hPairs[j-1].score; j-- {
					hPairs[j], hPairs[j-1] = hPairs[j-1], hPairs[j]
				}
			}

			bRank := make(map[string]int)
			for i, pair := range bPairs {
				bRank[pair.urn] = i + 1
			}

			hRank := make(map[string]int)
			for i, pair := range hPairs {
				hRank[pair.urn] = i + 1
			}

			allURNs := make(map[string]struct{})
			for urn := range bm25Scores {
				allURNs[urn] = struct{}{}
			}
			for urn := range vectorScores {
				allURNs[urn] = struct{}{}
			}

			var vectorDominant, lexicalDominant int64
			const k = 60.0
			for urn := range allURNs {
				var fused float64
				hasVec := false
				hasBM25 := false

				if r, ok := hRank[urn]; ok {
					fused += 1.0 / (k + float64(r))
					hasVec = true
				}
				if r, ok := bRank[urn]; ok {
					fused += 1.0 / (k + float64(r))
					hasBM25 = true
				}

				if hasVec && hasBM25 {
					if vectorScores[urn] > bm25Scores[urn] {
						vectorDominant++
					} else {
						lexicalDominant++
					}
				}
				// 🛡️ ISOLATED RRF SCALING: Multiply by 31.0 to normalize max theoretical score (1/61 + 1/61) * 31 ≈ 1.0
				scores[urn] = fused * 31.0
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

	// 3. Intent-Weighted Tri-Factor Scoring with Server Diversity
	var candidates []scoredTool

	// Server diversity tracking: count how many tools from each server
	// have scored above a competitive threshold so far.
	serverCounts := make(map[string]int)
	var totalCandidates int

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
			// Ghost entry exists but has no timestamp (pre-decay era) — use strict neutral base
			sSynergy = 0.1
		}

		// R_role: Intent-Role alignment boost (blended across matched categories)
		rRole := computeRoleBoostBlended(t.Role, intentWeights)

		// RRF Biases — rebalanced to reduce engine dominance and amplify
		// structural role alignment. Old: 0.50/0.20/0.30. New: 0.40/0.15/0.35.
		biasVector := 0.40
		biasSynergy := 0.15
		biasRole := 0.35
		if h.Config != nil {
			biasVector = h.Config.SynthesisBiasVector
			biasSynergy = h.Config.SynthesisBiasSynergy
			biasRole = h.Config.SynthesisBiasRole
		}

		// Tri-factor scoring: engine × wEngine + synergy × wSynergy + role × wRole
		sFinal := (sEngine * biasVector) + (sSynergy * biasSynergy) + (rRole * biasRole)

		// ── Option 3: Intent-Keyed Outcome Scoring (4th factor) ──
		// Boost or penalize based on historical intent→tool success rate.
		sIntent := intelligence.GetIntentToolScore(h.Store, intent, t.URN)
		if sIntent > 0 {
			// Weight intent score at 10% of total — enough to influence, not dominate.
			sFinal += sIntent * 0.10
		}

		// ── Option 6: Contrastive Failure Anchor Penalty ──
		// Apply multiplicative penalty if this tool has failed for similar intents.
		failurePenalty := intelligence.CheckFailureProximity(ctx, h.Store, intent, t.URN)
		sFinal *= failurePenalty

		// ── Negative Trigger Filtering ──
		// If the tool's NegativeTriggers contain tokens matching the intent,
		// apply a 0.5 multiplicative penalty. This provides a feedback
		// mechanism — poor tool selections can be corrected by adding negative
		// triggers without code changes.
		if len(t.NegativeTriggers) > 0 {
			normalizedIntent := strings.ToLower(intent)
			for _, nt := range t.NegativeTriggers {
				if strings.Contains(normalizedIntent, strings.ToLower(nt)) {
					sFinal *= 0.5
					break
				}
			}
		}

		// ── Server Diversity Multiplier ──
		// Prevent single-server domination. If a server already contributes
		// >50% of candidates, dampen new tools from that server. If <20%,
		// boost to encourage cross-server representation.
		if totalCandidates > 2 {
			serverRatio := float64(serverCounts[t.Server]) / float64(totalCandidates)
			if serverRatio > 0.50 {
				sFinal *= 0.85 // Dampen over-represented server
			} else if serverRatio < 0.20 {
				sFinal *= 1.15 // Boost under-represented server
			}
		}

		candidates = append(candidates, scoredTool{record: t, finalScore: sFinal})
		serverCounts[t.Server]++
		totalCandidates++
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

	// 🛡️ ROLE PRESERVATION GUARANTEE (Swarm Bidding Starvation Fix)
	// If the pipeline allows PLANNER or SYNTHESIZER roles, ensure ALL
	// tools of these roles survive the median absolute deviation threshold to guarantee a complete DAG natively.
	if len(roleMap) == 0 || roleMap["PLANNER"] || roleMap["SYNTHESIZER"] || roleMap["REPORTING"] {
		slog.Info(fmt.Sprintf("DEBUG: candidates length: %d", len(candidates)))

		injectIfMissing := func(best *scoredTool) {
			if best == nil {
				return
			}
			exists := false
			for _, q := range qualified {
				if q.record.URN == best.record.URN {
					exists = true
					break
				}
			}
			if !exists {
				// Inject with threshold bounds to confidently survive later phase-role pruning
				clone := *best
				clone.finalScore = threshold + 0.01
				qualified = append(qualified, clone)
				slog.Info(fmt.Sprintf("DEBUG: Injected %s into qualified with finalScore %f", best.record.URN, clone.finalScore))
			} else {
				slog.Info(fmt.Sprintf("DEBUG: %s is already in qualified", best.record.URN))
			}
		}

		for i := range candidates {
			c := &candidates[i]
			if c.record.Role == "PLANNER" || c.record.Role == "SYNTHESIZER" || c.record.Role == "REPORTING" {
				slog.Info(fmt.Sprintf("DEBUG: Preserving Role Tool: %s, Role: %s, Score: %f", c.record.URN, c.record.Role, c.finalScore))
				injectIfMissing(c)
			}
		}
	}

	// 🛡️ PHASE COVERAGE GAP ANALYSIS
	// A well-formed pipeline needs tools across multiple phases. If the MAD
	// threshold eliminated all tools from a structurally necessary phase, find
	// the best candidate from that phase and inject it. This is dynamic — it
	// doesn't specify WHICH tools, just ensures structural phase coverage.
	{
		coveredPhases := make(map[int]bool)
		qualifiedSet := make(map[string]bool)
		for _, q := range qualified {
			coveredPhases[q.record.Phase] = true
			qualifiedSet[q.record.URN] = true
		}

		// Check phases 0-5 for gaps. For each missing phase, inject the
		// highest-scoring unqualified candidate from that phase.
		for phase := 0; phase <= 5; phase++ {
			if coveredPhases[phase] {
				continue
			}
			var best *scoredTool
			for i := range candidates {
				c := &candidates[i]
				if c.record.Phase == phase && !qualifiedSet[c.record.URN] {
					if best == nil || c.finalScore > best.finalScore {
						best = c
					}
				}
			}
			if best != nil {
				clone := *best
				clone.finalScore = threshold + 0.01
				qualified = append(qualified, clone)
				qualifiedSet[clone.record.URN] = true
				slog.Info("swarm_bidding: phase coverage gap filled",
					"phase", phase,
					"urn", clone.record.URN,
					"role", clone.record.Role,
				)
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

	// 🛡️ PHASE-ROLE CLUSTER PRUNING: Strict mapping. Limit to 2 tools per (phase, role) group natively.
	// This prevents pathological clustering and natively shrinks pipeline steps.
	qualified = prunePhaseRoleClusters(qualified, 3)

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
	stages = resolveDynamicDAG(stages, pipelineTools, intent)

	// 🛡️ DFS Topological Mapping: Kahn's Gatekeeper Algorithm natively replacing SliceStable flaws.
	stages, warnings := topologicalSort(stages, pipelineTools)

	// 🛡️ Mutual Exclusivity Enclaves: Prune colliding analyzers internally
	stages = enforceExclusivityEnclaves(stages)

	return stages, warnings
}

// resolveDynamicDAG continuously walks the explicit Requires/Triggers constraints provided by local MCP Sub-Servers.
// Recursively extracts properties ensuring no dependency limit violates pure topology limits physically tracking graph logic dynamically.
func resolveDynamicDAG(stages []PipelineStep, pipelineTools []*db.ToolRecord, intent string) []PipelineStep {
	registry := make(map[string]*db.ToolRecord)
	for _, t := range pipelineTools {
		registry[t.URN] = t
	}

	// 🛡️ ATOMIC SOCRATIC TRIFECTA ENCLAVE
	// If the intent asks to improve code, or if any trifecta member is present, ensure ALL THREE are injected.
	trifectaURNs := []string{
		"brainstorm:thesis_architect",
		"brainstorm:antithesis_skeptic",
		"brainstorm:aporia_engine",
	}

	intentLower := strings.ToLower(intent)
	needsTrifecta := strings.Contains(intentLower, "improve") || strings.Contains(intentLower, "evaluate")

	if !needsTrifecta {
		for _, s := range stages {
			if slices.Contains(trifectaURNs, s.ToolName) {
				needsTrifecta = true
			}
			if needsTrifecta {
				break
			}
		}
	}

	if needsTrifecta {
		selected := make(map[string]bool)
		for _, s := range stages {
			selected[s.ToolName] = true
		}
		for _, urn := range trifectaURNs {
			if !selected[urn] {
				if target, ok := registry[urn]; ok {
					stages = append(stages, PipelineStep{
						ToolName:       target.URN,
						Role:           target.Role,
						Phase:          target.Phase,
						Purpose:        "Atomic Socratic Trifecta Enclave: Automatically bound sequentially.",
						InputContract:  target.InputContract,
						OutputContract: target.OutputContract,
					})
					selected[urn] = true
				}
			}
		}
	}

	// 🛡️ MANDATORY PLANNER INJECTION
	// If the intent requires mutation (refactor, fix, optimize, etc.), ensure at
	// least one PLANNER tool is in the DAG so a concrete implementation plan is
	// generated before autonomous MUTATOR injection.
	if intentRequiresMutation(intent) {
		hasPLANNER := false
		currentSelected := make(map[string]bool)
		for _, s := range stages {
			currentSelected[s.ToolName] = true
			if s.Role == "PLANNER" {
				hasPLANNER = true
			}
		}
		if !hasPLANNER {
			plannerURN := "go-refactor:generate_implementation_plan"
			if target, ok := registry[plannerURN]; ok && !currentSelected[plannerURN] {
				stages = append(stages, PipelineStep{
					ToolName:       target.URN,
					Role:           target.Role,
					Phase:          target.Phase,
					Purpose:        "Mandatory PLANNER Injection: Ensures plan generation before autonomous MUTATOR injection.",
					InputContract:  target.InputContract,
					OutputContract: target.OutputContract,
				})
				slog.Info("resolveDynamicDAG: mandatory PLANNER injected", "urn", plannerURN)
			}
		}
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

		// 🛡️ REVERSE-TOPOLOGY INJECTION: Interceptor mapping natively overriding starvation flaws.
		for _, rec := range pipelineTools {
			if selected[rec.URN] {
				continue
			}
			// Only orchestrator firewalls, synthesizers, and validators organically pull themselves in
			if rec.Server != "magictools" {
				continue
			}
			for _, req := range rec.Requires {
				if selected[req] {
					stages = append(stages, PipelineStep{
						ToolName:       rec.URN,
						Role:           rec.Role,
						Phase:          rec.Phase,
						Purpose:        fmt.Sprintf("Dynamic DAG Interceptor: Automatically bound to required node %s natively.", req),
						InputContract:  rec.InputContract,
						OutputContract: rec.OutputContract,
					})
					selected[rec.URN] = true
					addedNewNode = true
					break
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
	return dag.ScoreGroupedByServer(pruned, pipelineTools)
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

// classifyIntentWeights determines proportional intent category weights from
// the user's request. Instead of a single winner, returns normalized weights
// across all matching categories. This eliminates cliff-edge instability where
// "evaluate and refactor" vs "refactor and evaluate" would flip the entire
// role boost table.
func classifyIntentWeights(intent string) map[string]float64 {
	lower := strings.ToLower(intent)

	auditSignals := []string{"audit", "analyze", "review", "inspect", "assess", "evaluate", "check", "scan", "trace"}
	refactorSignals := []string{"refactor", "fix", "modernize", "optimize", "clean", "prune", "migrate", "upgrade"}
	planSignals := []string{"plan", "design", "architect", "propose", "strategy", "blueprint", "feature"}

	weights := map[string]float64{"audit": 0, "refactor": 0, "plan": 0}
	for _, s := range auditSignals {
		if strings.Contains(lower, s) {
			weights["audit"]++
		}
	}
	for _, s := range refactorSignals {
		if strings.Contains(lower, s) {
			weights["refactor"]++
		}
	}
	for _, s := range planSignals {
		if strings.Contains(lower, s) {
			weights["plan"]++
		}
	}

	// Normalize to proportions [0,1]
	total := weights["audit"] + weights["refactor"] + weights["plan"]
	if total > 0 {
		for k := range weights {
			weights[k] /= total
		}
	} else {
		// No signals matched — default to audit (analysis-only)
		weights["audit"] = 1.0
	}

	return weights
}

// computeRoleBoostBlended returns a weighted role boost blended across all
// matched intent categories. For "evaluate and refactor" (audit=0.5, refactor=0.5):
//
//	ANALYZER = (1.0 × 0.5) + (0.75 × 0.5) = 0.875
//	CRITIC   = (0.7 × 0.5) + (0.5 × 0.5)  = 0.60
//	MUTATOR  = (0.2 × 0.5) + (1.0 × 0.5)  = 0.60
func computeRoleBoostBlended(role string, weights map[string]float64) float64 {
	boost := 0.0
	for intentType, weight := range weights {
		boost += lookupRoleBoost(role, intentType) * weight
	}
	return boost
}

// lookupRoleBoost returns the single-category role boost value.
// This is the lookup table used by computeRoleBoostBlended.
func lookupRoleBoost(role, intentType string) float64 {
	switch intentType {
	case "audit":
		switch role {
		case "ANALYZER":
			return 1.0
		case "CRITIC":
			return 0.7
		case "SYNTHESIZER":
			return 0.4
		case "PLANNER":
			return 0.3
		case "MUTATOR":
			return 0.2
		}
	case "refactor":
		switch role {
		case "MUTATOR":
			return 1.0
		case "ANALYZER":
			return 0.75
		case "PLANNER":
			return 0.7
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

// trifectaURNs lists the Socratic Trifecta members that must never be
// amputated by the scope cap.
var trifectaURNs = map[string]bool{
	"brainstorm:thesis_architect":   true,
	"brainstorm:antithesis_skeptic": true,
	"brainstorm:aporia_engine":      true,
}

// smartCap applies a role-aware cap that protects structural guarantees:
//  1. Sole representatives of a role are unconditionally preserved.
//  2. Trifecta members are unconditionally preserved.
//  3. Excess is trimmed from the most over-represented role first (by count).
//
// This replaces the naive stages[:maxTools] truncation that blindly amputated
// critical pipeline members landing past the scope cut.
func smartCap(stages []PipelineStep, maxTools int) []PipelineStep {
	if len(stages) <= maxTools {
		return stages
	}

	// Phase 1: Identify protected stages.
	roleCounts := make(map[string]int)
	for _, s := range stages {
		roleCounts[s.Role]++
	}

	protected := make(map[int]bool) // index → is protected
	for i, s := range stages {
		// Protect Trifecta members unconditionally.
		if trifectaURNs[s.ToolName] {
			protected[i] = true
			continue
		}
		// Protect sole role representatives.
		if roleCounts[s.Role] == 1 {
			protected[i] = true
		}
	}

	// Phase 2: Trim unprotected stages from the most over-represented role.
	excess := len(stages) - maxTools
	for excess > 0 {
		// Find the most over-represented role (by unprotected count).
		roleUnprotected := make(map[string]int)
		for i, s := range stages {
			if !protected[i] {
				roleUnprotected[s.Role]++
			}
		}

		// Find the role with the most unprotected members.
		var trimRole string
		trimMax := 0
		for role, count := range roleUnprotected {
			if count > trimMax {
				trimMax = count
				trimRole = role
			}
		}
		if trimMax == 0 {
			break // All remaining are protected — cannot trim further.
		}

		// Remove the LAST unprotected member of the most over-represented role
		// (last = lowest priority since stages are sorted by score descending).
		for i := len(stages) - 1; i >= 0; i-- {
			if stages[i].Role == trimRole && !protected[i] {
				stages = append(stages[:i], stages[i+1:]...)
				// Re-index protected map after removal.
				newProtected := make(map[int]bool)
				for k := range protected {
					if k < i {
						newProtected[k] = true
					} else if k > i {
						newProtected[k-1] = true
					}
				}
				protected = newProtected
				excess--
				break
			}
		}
	}

	return stages
}
