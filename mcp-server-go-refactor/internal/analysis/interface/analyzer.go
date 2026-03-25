package interfaceanalysis

import (
	"context"
	"fmt"
	"go/types"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tool implements the interface discovery tool.
type Tool struct{}

// Register adds the interface discovery tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_interface_discovery",
		mcp.WithDescription("Analyzes the structural signatures of exported types to find common method patterns across multiple structs. Use this to discover hidden abstractions and suggest new interface definitions that can decouple components and improve testability via mocking."),
		mcp.WithString("pkg", mcp.Description("The package path"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	suggestions, err := DiscoverSharedInterfaces(ctx, pkg)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if len(suggestions) == 0 {
		return mcp.NewToolResultText("No shared structural signatures discovered."), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%+v", suggestions)), nil
}

type interfaceSuggestion struct {
	Structs []string `json:"Structs"`
	Methods []string `json:"Methods"`
}

func DiscoverSharedInterfaces(ctx context.Context, pkgPath string) ([]interfaceSuggestion, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	allStructs := make(map[string][]string)
	for _, pkg := range pkgs {
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if _, ok := obj.Type().Underlying().(*types.Struct); ok {
				// Get methods of the struct (including pointer receivers)
				ptr := types.NewPointer(obj.Type())
				ms := types.NewMethodSet(ptr)
				methods := []string{}
				for i := 0; i < ms.Len(); i++ {
					m := ms.At(i).Obj().(*types.Func)
					if m.Exported() {
						methods = append(methods, m.Name())
					}
				}
				if len(methods) > 0 {
					allStructs[name] = methods
				}
			}
		}
	}

	// Simple clustering: group structs that share more than 2 methods
	suggestions := []interfaceSuggestion{}
	processed := make(map[string]bool)

	structNames := []string{}
	for k := range allStructs {
		structNames = append(structNames, k)
	}

	for i := 0; i < len(structNames); i++ {
		if processed[structNames[i]] {
			continue
		}
		name1 := structNames[i]
		methods1 := allStructs[name1]
		
		group := []string{name1}
		commonMethods := methods1

		for j := i + 1; j < len(structNames); j++ {
			name2 := structNames[j]
			methods2 := allStructs[name2]
			
			intersect := intersection(commonMethods, methods2)
			if len(intersect) >= 2 {
				group = append(group, name2)
				commonMethods = intersect
				processed[name2] = true
			}
		}

		if len(group) > 1 {
			suggestions = append(suggestions, interfaceSuggestion{
				Structs: group,
				Methods: commonMethods,
			})
		}
	}

	return suggestions, nil
}

func intersection(s1, s2 []string) []string {
	m := make(map[string]bool)
	for _, v := range s1 {
		m[v] = true
	}
	res := []string{}
	for _, v := range s2 {
		if m[v] {
			res = append(res, v)
		}
	}
	return res
}
