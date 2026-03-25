package astutil

import (
	"context"
	"fmt"
	"go/types"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/tools/go/packages"
)

// Tool implements the interface analyzer tool.
type Tool struct{}

// Register adds the interface analytical tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_interface_tool",
		mcp.WithDescription("A dual-purpose utility for interface management. It can either extract a complete interface definition from an existing struct or perform a rigorous compatibility check between a struct and an interface. Use this when defining new service layers or verifying that a refactored struct still satisfies its expected contracts."),
		mcp.WithString("pkg", mcp.Description("The package path"), mcp.Required()),
		mcp.WithString("structName", mcp.Description("The name of the struct"), mcp.Required()),
		mcp.WithString("ifaceName", mcp.Description("The name of the interface to check against (optional). If omitted, extracts a new interface from the struct.")),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	structName := request.GetString("structName", "")
	ifaceName := request.GetString("ifaceName", "")

	if ifaceName != "" {
		result, err := AnalyzeInterface(ctx, pkg, structName, ifaceName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Interface Compatibility Analysis for %s:%s\n%+v", structName, ifaceName, result)), nil
	}

	result, err := ExtractInterface(ctx, pkg, structName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Extracted Interface Definition:\n%+v", result)), nil
}

type Analysis struct {
	StructName     string   `json:"StructName"`
	InterfaceName  string   `json:"InterfaceName"`
	MissingMethods []string `json:"MissingMethods"`
	IsCompatible   bool     `json:"IsCompatible"`
}

// AnalyzeInterface compares a struct's methods to an interface's defined methods.
func AnalyzeInterface(ctx context.Context, pkgPath string, structName string, ifaceName string) (*Analysis, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no package found at: %s", pkgPath)
	}

	var obj types.Object
	var targetPkg *packages.Package
	for _, p := range pkgs {
		if o := p.Types.Scope().Lookup(structName); o != nil {
			obj = o
			targetPkg = p
			break
		}
	}

	if obj == nil {
		return nil, fmt.Errorf("struct %s not found in any loaded packages of %s", structName, pkgPath)
	}

	ifaceObj := targetPkg.Types.Scope().Lookup(ifaceName)
	if ifaceObj == nil {
		return nil, fmt.Errorf("interface %s not found in same package as struct %s", ifaceName, structName)
	}

	iface, ok := ifaceObj.Type().Underlying().(*types.Interface)
	if !ok {
		return nil, fmt.Errorf("%s is not an interface", ifaceName)
	}

	// Pointer receiver check
	ptr := types.NewPointer(obj.Type())
	missing := []string{}

	for i := 0; i < iface.NumMethods(); i++ {
		m := iface.Method(i)
		selection, _, _ := types.LookupFieldOrMethod(ptr, true, targetPkg.Types, m.Name())
		if selection == nil {
			missing = append(missing, m.FullName())
		}
	}

	return &Analysis{
		StructName:     structName,
		InterfaceName:  ifaceName,
		MissingMethods: missing,
		IsCompatible:   len(missing) == 0,
	}, nil
}

// ExtractionResult details the generated interface.
type ExtractionResult struct {
	InterfaceName string   `json:"InterfaceName"`
	Methods       []string `json:"Methods"`
}

// ExtractInterface identifies all public methods of a struct to define an interface.
func ExtractInterface(ctx context.Context, pkgPath string, structName string) (*ExtractionResult, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	var obj types.Object
	var targetPkg *packages.Package
	for _, p := range pkgs {
		if o := p.Types.Scope().Lookup(structName); o != nil {
			obj = o
			targetPkg = p
			break
		}
	}

	if obj == nil {
		return nil, fmt.Errorf("struct %s not found", structName)
	}

	ptr := types.NewPointer(obj.Type())
	methods := []string{}

	// Iterate through all methods (including pointer receivers)
	ms := types.NewMethodSet(ptr)
	for i := 0; i < ms.Len(); i++ {
		m := ms.At(i).Obj().(*types.Func)
		// Need go/ast for IsExported which we can import
		if m.Exported() {
			sig := m.Type().(*types.Signature)
			sigStr := types.TypeString(sig, func(p *types.Package) string {
				if p == targetPkg.Types {
					return ""
				}
				return p.Name()
			})
			methods = append(methods, fmt.Sprintf("%s%s", m.Name(), strings.TrimPrefix(sigStr, "func")))
		}
	}

	return &ExtractionResult{
		InterfaceName: "I" + structName,
		Methods:       methods,
	}, nil
}
