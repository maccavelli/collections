package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"
	"mcp-server-magictools/internal/db"
	"mcp-server-magictools/internal/vector"
)

type similarityResult struct {
	ToolA           string
	ToolB           string
	BleveScore      float64
	StructuralScore float64
	TotalScore      float64
	Category        string
	Recommendation  string
}

// extractKeys recurses a JSON schema map to collect all structural keys and their types.
func extractKeys(prefix string, schema map[string]any) map[string]string {
	keys := make(map[string]string)
	if schema == nil {
		return keys
	}
	for k, v := range schema {
		typeStr := "unknown"
		if vm, ok := v.(map[string]any); ok {
			if t, ok := vm["type"].(string); ok {
				typeStr = t
			}
			// Recurse into properties
			if props, ok := vm["properties"].(map[string]any); ok {
				subKeys := extractKeys(prefix+k+".", props)
				for sk, sv := range subKeys {
					keys[sk] = sv
				}
			}
		}
		keys[prefix+k] = typeStr
	}
	return keys
}

// computeJaccard calculates Jaccard similarity of two string maps.
func computeJaccard(mapA, mapB map[string]string) float64 {
	if len(mapA) == 0 && len(mapB) == 0 {
		return 1.0 // Both perfectly empty
	}
	intersection := 0
	for k, va := range mapA {
		if vb, ok := mapB[k]; ok && va == vb {
			intersection++
		}
	}
	union := len(mapA) + len(mapB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// SemanticSimilarityAudit is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) SemanticSimilarityAudit(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slog.Info("semantic_similarity: starting full registry audit (Muzzle Protocol: os.Stderr)", "workers", 4)

	// Print directly to Stderr to fulfill Muzzle Protocol
	slog.Info("starting auditory analysis using 4 bounded threads", "component", "semantic_similarity")

	// Parse optional target servers constraint
	var args map[string]any
	targetServers := make(map[string]bool)
	var artifactPath string
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err == nil {
			if serversArg, ok := args["servers"].(string); ok && serversArg != "" {
				for _, s := range strings.Fields(serversArg) {
					targetServers[s] = true
				}
			}
			if ap, ok := args["artifact_path"].(string); ok {
				artifactPath = strings.TrimSpace(ap)
			}
		}
	}

	// Phase I: Get ALL tools
	allTools, err := h.Store.SearchTools("", "", "", 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list all tools: %w", err)
	}

	if len(allTools) == 0 {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "No tools found in registry."}}}, nil
	}

	resultsChan := make(chan []similarityResult, len(allTools))
	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(4) // 🛡️ Bounded background worker pattern

	for _, t := range allTools {
		toolA := t // Capture loop variable
		if len(targetServers) > 0 && !targetServers[toolA.Server] {
			continue // Optional scope bypass (outer loop cache save)
		}

		eg.Go(func() error {
			if toolA.Description == "" {
				return nil
			}

			// 🛡️ NATIVE SPLIT-BRAIN: Use Cosine Semantic Search globally if active
			e := vector.GetEngine()
			var matches []*db.ToolRecord
			var err error

			if e != nil && e.VectorEnabled() {
				scoredNodes, sErr := e.SearchByNode(ctx, toolA.URN, 10)
				if sErr == nil && len(scoredNodes) > 0 {
					for _, node := range scoredNodes {
						if tr, rErr := h.Store.GetTool(node.Key); rErr == nil {
							// Translate verified actual Cosine Similarity mathematically natively
							tr.ConfidenceScore = node.Score
							matches = append(matches, tr)
						}
					}
				} else {
					matches, err = h.Store.SearchTools(toolA.Description, toolA.Category, "", 0.7)
				}
			} else {
				// OFFLINE: Bleve Natural Linguistic Search
				matches, err = h.Store.SearchTools(toolA.Description, toolA.Category, "", 0.7)
			}

			if err != nil || len(matches) == 0 {
				return nil
			}

			// Extract Tool A schema keys
			keysA := extractKeys("", toolA.InputSchema)

			var localResults []similarityResult
			for _, toolB := range matches {
				// Prevent self-matches explicitly by URN
				if toolB.URN == toolA.URN {
					continue
				}

				if len(targetServers) > 0 && !targetServers[toolB.Server] {
					continue // Optional scope bypass (inner loop structural save)
				}

				// Deduplicate structural checks (A->B is same as B->A, but we only record what Bleve finds directionally)

				// Phase II: Structural fingerprinting
				keysB := extractKeys("", toolB.InputSchema)
				structuralScore := computeJaccard(keysA, keysB)

				// 🛡️ Option A: Schema Discount Penalty
				// If both tools rely on implicit state or generic structures (< 2 properties), penalize to avoid 100% false positives
				if len(keysA) < 2 && len(keysB) < 2 {
					structuralScore = 0.0
				}

				// Phase III: Merge ROI Formula (Option A: Linguistic Primacy)
				// SimScore = (Bleve * 0.8) + (Structural * 0.2)

				// Bleve's ConfidenceScore is unnormalized (TF-IDF often > 1.0).
				// We cap it mathematically for the ROI formula.
				bleveNorm := math.Min(1.0, toolB.ConfidenceScore)

				roi := (bleveNorm * 0.8) + (structuralScore * 0.2)

				if roi > 0.5 { // Only bother saving if > 0.5
					category := "Shared Domain: Consider Shared Utilities"
					if roi > 0.9 {
						category = "Redundant Duplicate: Immediate Refactor"
					} else if roi >= 0.7 {
						category = "Functional Overlap: Merge Candidate"
					}

					localResults = append(localResults, similarityResult{
						ToolA:           toolA.URN,
						ToolB:           toolB.URN,
						BleveScore:      toolB.ConfidenceScore,
						StructuralScore: structuralScore,
						TotalScore:      roi,
						Recommendation:  category,
					})

					// 🛡️ Muzzle Protocol metrics logging
					fmt.Fprintf(os.Stderr, "[semantic_similarity] MATCH FOUND | %s <-> %s | Bleve: %.2f | Struct: %.2f | ROI: %.2f\n",
						toolA.URN, toolB.URN, toolB.ConfidenceScore, structuralScore, roi)
				}
			}

			if len(localResults) > 0 {
				resultsChan <- localResults
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "[semantic_similarity] Worker pool error: %v\n", err)
	}
	close(resultsChan)

	// Consolidate matches (removing A->B vs B->A duplicates based on alphabetical URN sorting)
	uniqueMatches := make(map[string]similarityResult)
	for resBatch := range resultsChan {
		for _, res := range resBatch {
			a, b := res.ToolA, res.ToolB
			if a > b {
				a, b = b, a
			}
			key := a + ":::" + b
			if existing, ok := uniqueMatches[key]; ok {
				// Keep the one with higher total score if slight score fluctuations exist
				if res.TotalScore > existing.TotalScore {
					uniqueMatches[key] = res
				}
			} else {
				uniqueMatches[key] = res
			}
		}
	}

	// Phase IV: Markdown generation
	var summary strings.Builder
	summary.WriteString("# Semantic Similarity Audit\n\n")

	if len(uniqueMatches) == 0 {
		summary.WriteString("*No overlapping tools found matching the threshold.*")
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: summary.String()}}}, nil
	}

	// Sort results by ROI descending
	var sorted []similarityResult
	for _, v := range uniqueMatches {
		sorted = append(sorted, v)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TotalScore > sorted[j].TotalScore
	})

	summary.WriteString("| Tool A | Tool B | Similarity | Socratic Recommendation |\n")
	summary.WriteString("| :--- | :--- | :--- | :--- |\n")
	for _, res := range sorted {
		scorePct := int(res.TotalScore * 100)
		summary.WriteString(fmt.Sprintf("| %s | %s | %d%% | %s |\n", res.ToolA, res.ToolB, scorePct, res.Recommendation))
	}

	markdown := summary.String()
	// 🚀 ARTIFACT FAST-PATH: Write directly to disk when artifact_path is provided
	if artifactPath != "" {
		if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create artifact directory: %w", err)
		}
		if err := os.WriteFile(artifactPath, []byte(markdown), 0o644); err != nil {
			return nil, fmt.Errorf("failed to write artifact: %w", err)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Artifact written to: %s", artifactPath)}}}, nil
	}

	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: markdown}}}, nil
}
