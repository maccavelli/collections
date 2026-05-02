package memory

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MemoryAnalysisResult aggregates findings from all memory sub-analyzers.
type MemoryAnalysisResult struct {
	GoroutineLeaks   []Finding `json:"goroutine_leaks"`
	ResourceLeaks    []Finding `json:"resource_leaks"`
	AllocationIssues []Finding `json:"allocation_issues"`
	EscapeHints      []Finding `json:"escape_hints,omitzero"`
	SyncPoolIssues   []Finding `json:"sync_pool_issues,omitzero"`
	ModernPatterns   []Finding `json:"modern_patterns,omitzero"`
}

// TotalFindings returns the total count across all categories.
func (r *MemoryAnalysisResult) TotalFindings() int {
	return len(r.GoroutineLeaks) + len(r.ResourceLeaks) + len(r.AllocationIssues) +
		len(r.EscapeHints) + len(r.SyncPoolIssues) + len(r.ModernPatterns)
}

// Tool implements the go_memory_analyzer tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_memory_analyzer"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name: t.Name(),
		Description: "[ROLE: ANALYZER] MEMORY SAFETY AUDITOR: Comprehensive memory analysis suite " +
			"detecting goroutine leaks, resource leaks, unbounded allocations, escape analysis " +
			"hints, sync.Pool misuse, and Go 1.26 memory hardening compliance. Produces " +
			"categorized finding list with severity and remediation guidance. " +
			"[REQUIRES: go-refactor:go_ast_suite_analyzer] " +
			"[TRIGGERS: brainstorm:critique_design] " +
			"[Routing Tags: memory, leak, goroutine-leak, resource-leak, allocation, " +
			"escape-analysis, sync-pool, gc-pressure, heap, memory-safety]",
	}, t.Handle)
}

// Register adds the memory analyzer tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

// MemoryInput defines the expected arguments.
type MemoryInput struct {
	models.UniversalPipelineInput
}

func (t *Tool) Handle(ctx context.Context, _ *mcp.CallToolRequest, input MemoryInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Target)

		if recallAvailable {
			if history := t.Engine.LoadCrossSessionFromRecall(ctx, "gorefactor", input.Target); history != "" {
				if session.Metadata == nil {
					session.Metadata = make(map[string]any)
				}
				session.Metadata["historical_memory"] = history
			}
		}
	}

	// Load all packages once — shared across all sub-analyzers.
	pkgs, err := loader.LoadPackages(ctx, input.Target, loader.DefaultMode)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	// Execute all sub-analyzers against the same loaded packages.
	result := &MemoryAnalysisResult{
		GoroutineLeaks:   DetectGoroutineLeaks(pkgs),
		ResourceLeaks:    DetectResourceLeaks(pkgs),
		AllocationIssues: DetectAllocationIssues(pkgs),
		EscapeHints:      DetectEscapeIssues(pkgs),
		SyncPoolIssues:   DetectSyncPoolIssues(pkgs),
		ModernPatterns:   DetectModernPatterns(pkgs),
	}

	total := result.TotalFindings()
	summary := "No memory issues found."
	if total > 0 {
		summary = formatSummary(input.Target, result)
	}

	// Session and recall integration.
	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			memStds := t.Engine.EnsureRecallCache(ctx, session, "memory_safety", "search", map[string]any{
				"namespace": "ecosystem",
				"query":     "Go memory safety standards, goroutine leak patterns, resource management, sync.Pool best practices, escape analysis optimization for " + input.Target,
				"limit":     15,
			})
			session.Metadata["recall_cache_memory"] = memStds

			if memStds != "" {
				summary += fmt.Sprintf("\n\n[Memory Safety Standards]: %s", memStds)
			}
		}

		// Pillar metrics for brainstorm learning.
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":              "memory_safety",
			"goroutine_leaks":    len(result.GoroutineLeaks),
			"resource_leaks":     len(result.ResourceLeaks),
			"allocation_issues":  len(result.AllocationIssues),
			"escape_hints":       len(result.EscapeHints),
			"sync_pool_issues":   len(result.SyncPoolIssues),
			"modern_patterns":    len(result.ModernPatterns),
			"total_findings":     total,
		}

		if total > 0 {
			var astFaults []string
			if f, ok := session.Metadata["ast_faults"].([]string); ok {
				astFaults = f
			}
			session.Metadata["ast_faults"] = append(astFaults, "memory_violation")
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "memory_analyzed", "native", "go_memory_analyzer", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string                `json:"summary"`
		Data    *MemoryAnalysisResult `json:"data"`
	}{
		Summary: summary,
		Data:    result,
	}, nil
}

// formatSummary builds a human-readable summary from the analysis result.
func formatSummary(target string, r *MemoryAnalysisResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memory issues in package %s:\n", r.TotalFindings(), target))

	if len(r.GoroutineLeaks) > 0 {
		sb.WriteString(fmt.Sprintf("  - %d goroutine leak(s)\n", len(r.GoroutineLeaks)))
	}
	if len(r.ResourceLeaks) > 0 {
		sb.WriteString(fmt.Sprintf("  - %d resource leak(s)\n", len(r.ResourceLeaks)))
	}
	if len(r.AllocationIssues) > 0 {
		sb.WriteString(fmt.Sprintf("  - %d allocation issue(s)\n", len(r.AllocationIssues)))
	}
	if len(r.EscapeHints) > 0 {
		sb.WriteString(fmt.Sprintf("  - %d escape analysis hint(s)\n", len(r.EscapeHints)))
	}
	if len(r.SyncPoolIssues) > 0 {
		sb.WriteString(fmt.Sprintf("  - %d sync.Pool issue(s)\n", len(r.SyncPoolIssues)))
	}
	if len(r.ModernPatterns) > 0 {
		sb.WriteString(fmt.Sprintf("  - %d Go 1.26 modernization(s)\n", len(r.ModernPatterns)))
	}
	return sb.String()
}
