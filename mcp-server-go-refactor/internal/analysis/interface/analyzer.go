package interfaceanalysis

import (
	"context"
	"fmt"
	"go/types"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"
)

// Tool implements the interface discovery tool.
type Tool struct{}

func (t *Tool) Name() string {
	return "go_interface_discovery"
}

func (t *Tool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "ABSTRACTION MANDATE / INTERFACE DISCOVERY: Analyzes structural signatures to find common method patterns across multiple structs. Call this to suggest new interface definitions that decouple components. Cascades to go_interface_tool for extraction.",
	}, t.Handle)
}

// Register adds the interface discovery tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

type DiscoveryInput struct {
	Pkg string `json:"pkg" jsonschema:"The package path"`
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DiscoveryInput) (*mcp.CallToolResult, any, error) {
	suggestions, err := DiscoverSharedInterfaces(ctx, input.Pkg)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	if len(suggestions) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "No shared structural signatures discovered."}},
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%+v", suggestions)}},
	}, nil, nil
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

	var mu sync.Mutex
	allStructs := make(map[string][]string)
	g, gCtx := errgroup.WithContext(ctx)

	for _, p := range pkgs {
		pkg := p
		g.Go(func() error {
			scope := pkg.Types.Scope()
			for _, name := range scope.Names() {
				select {
				case <-gCtx.Done():
					return gCtx.Err()
				default:
				}

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
						mu.Lock()
						allStructs[name] = methods
						mu.Unlock()
					}
				}
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
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
