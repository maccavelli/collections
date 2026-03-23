package layout

import (
	"context"
	"fmt"
	"go/types"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tool implements the struct alignment analyzer tool.
type Tool struct{}

// Register adds the struct alignment tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_struct_alignment_optimizer",
		mcp.WithDescription("Detects wasted padding in structs and recommends optimal field ordering."),
		mcp.WithString("pkg", mcp.Description("The package path"), mcp.Required()),
		mcp.WithString("structName", mcp.Description("The name of the struct"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	structName := request.GetString("structName", "")
	result, err := AnalyzeStructAlignment(ctx, structName, pkg)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
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
