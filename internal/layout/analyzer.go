// Package layout provides functionality for the layout subsystem.
package layout

import (
	"context"
	"fmt"
	"go/types"
	"log/slog"
	"os"
	"sort"

	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool implements the struct alignment analyzer tool.
type Tool struct {
	Engine *engine.Engine
}

// Name performs the Name operation.
func (t *Tool) Name() string {
	return "go_struct_alignment_optimizer"
}

// Register performs the Register operation.
func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] STRUCT ALIGNMENT OPTIMIZER: Analyzes object sizes, padding spacing, and memory layout of Go structs to identify wasted space. Provides optimized field ordering that minimizes memory footprint. Produces struct size analysis with optimal field ordering. In orchestrator mode, publishes telemetry for cross-session tracking.",
	}, t.Handle)
}

// Register adds the struct alignment tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

// AlignmentInput defines the AlignmentInput structure.
type AlignmentInput struct {
	models.UniversalPipelineInput
}

// Handle performs the Handle operation.
func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input AlignmentInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Target)
	}

	if input.Context == "" {
		summary := "Skipped struct alignment (no struct name provided)"
		if session != nil {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			var diags []string
			if d, ok := session.Metadata["diagnostics"].([]string); ok {
				diags = d
			}
			session.Metadata["diagnostics"] = append(diags, summary)
			t.Engine.SaveSession(session)

			if recallAvailable {
				t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "alignment_optimized", "native", "go_struct_alignment_optimizer", "", session.Metadata)
			}
		}
		return &mcp.CallToolResult{}, struct {
			Summary string           `json:"summary"`
			Data    *AlignmentResult `json:"data"`
		}{
			Summary: summary,
			Data:    nil,
		}, nil
	}

	result, err := AnalyzeStructAlignment(ctx, input.Context, input.Target)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	summary := fmt.Sprintf("Struct alignment for %s: %d bytes (optimal %d bytes)", input.Context, result.CurrentSizeBytes, result.OptimalSizeBytes)

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			structStds := t.Engine.EnsureRecallCache(ctx, session, "struct_layout", "search", map[string]any{"namespace": "ecosystem",
				"query": "Go struct memory alignment optimization, field ordering standards, and cache-line padding conventions for " + input.Target,
				"limit": 10,
			})
			session.Metadata["recall_cache_struct"] = structStds

			if structStds != "" {
				summary += fmt.Sprintf("\n\n[Struct Layout Standards]: %s", structStds)
			}
		}

		// Pillar metrics for brainstorm learning.
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":        "efficiency",
			"current_bytes": result.CurrentSizeBytes,
			"optimal_bytes": result.OptimalSizeBytes,
			"wasted_bytes":  result.CurrentSizeBytes - result.OptimalSizeBytes,
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "alignment_optimized", "native", "go_struct_alignment_optimizer", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string           `json:"summary"`
		Data    *AlignmentResult `json:"data"`
	}{
		Summary: summary,
		Data:    result,
	}, nil
}

// AlignmentResult contains memory analysis for a struct.
type AlignmentResult struct {
	StructName       string   `json:"StructName"`
	CurrentSizeBytes int64    `json:"CurrentSizeBytes"`
	OptimalSizeBytes int64    `json:"OptimalSizeBytes"`
	OptimalOrdering  []string `json:"OptimalOrdering"`
}

// AnalyzeStructAlignment calculates current vs optimal memory layout for a struct.
func AnalyzeStructAlignment(ctx context.Context, structName string, pkgPath string) (*AlignmentResult, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	var obj types.Object
	for _, p := range pkgs {
		if o := p.Types.Scope().Lookup(structName); o != nil {
			obj = o
			break
		}
	}

	if obj == nil {
		return nil, fmt.Errorf("struct %s not found in any loaded packages of %s", structName, pkgPath)
	}

	st, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("%s is not a struct", structName)
	}

	maxAlign := int64(8)
	sizes := types.SizesFor("gc", "amd64")

	// Current Size
	currentSize := sizes.Sizeof(st)

	// Calculate Optimal Size by sorting fields by size/alignment descending
	type field struct {
		name  string
		size  int64
		align int64
	}
	fields := make([]field, st.NumFields())
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		fields[i] = field{
			name:  f.Name(),
			size:  sizes.Sizeof(f.Type()),
			align: sizes.Alignof(f.Type()),
		}
	}

	// Greedy sort: Largest alignment requirements first
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].align != fields[j].align {
			return fields[i].align > fields[j].align
		}
		return fields[i].size > fields[j].size
	})

	var optimalSize int64
	var currentAlign int64
	optimalOrdering := make([]string, len(fields))
	for i, f := range fields {
		optimalOrdering[i] = f.name
		offset := (optimalSize + f.align - 1) &^ (f.align - 1)
		optimalSize = offset + f.size
		if f.align > currentAlign {
			currentAlign = f.align
		}
	}
	// Final padding
	optimalSize = (optimalSize + maxAlign - 1) &^ (maxAlign - 1)

	return &AlignmentResult{
		StructName:       structName,
		CurrentSizeBytes: currentSize,
		OptimalSizeBytes: optimalSize,
		OptimalOrdering:  optimalOrdering,
	}, nil
}
