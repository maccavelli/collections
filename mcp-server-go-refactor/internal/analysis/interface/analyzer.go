package interfaceanalysis

import (
	"context"
	"fmt"
	"go/types"
	"log/slog"
	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"

	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"
	"os"
	"sync"

	"mcp-server-go-refactor/internal/models"
	"strings"

	"github.com/tidwall/gjson"
	"golang.org/x/sync/errgroup"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool implements the interface discovery tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_interface_discovery"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] INTERFACE DISCOVERY: AST-driven scan for shared method signatures across structs in a package. Finds, locates, and groups types with 2+ common exported methods into candidate interface clusters. Use FIRST in the interface refactoring pipeline. [TRIGGERS: go-refactor:find_interface_implementations, brainstorm:peer_review] [Routing Tags: find-interfaces, abstract-types, structure-interfaces, decouple]",
	}, t.Handle)
}

// ImplementationTool identifies all types in a workspace that implement a specific interface.
type ImplementationTool struct {
	server *mcp.Server
	Engine *engine.Engine
}

func (t *ImplementationTool) Name() string {
	return "find_interface_implementations"
}

func (t *ImplementationTool) Register(s util.SessionProvider) {
	t.server = s.MCPServer()
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] IMPLEMENTATION MAPPER: Given a named interface (e.g., 'io.Reader'), searches, locates, and finds ALL concrete types satisfying it across the workspace via types.Implements. Requires 'context' field to specify the interface name. Produces implementation match list with package paths. In orchestrator mode, publishes telemetry. [Routing Tags: find-implementations, types.Implements, mapper, concrete-types]",
	}, t.Handle)
}

// Register adds both interface discovery and implementation finder tools to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
	registry.Global.Register(&ImplementationTool{Engine: eng})
}

type DiscoveryInput struct {
	models.UniversalPipelineInput
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DiscoveryInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Target)
	}

	suggestions, err := DiscoverSharedInterfaces(ctx, input.Target)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	summary := "No shared structural signatures discovered."
	if len(suggestions) > 0 {
		summary = fmt.Sprintf("Discovered %d potential shared interfaces in %s", len(suggestions), input.Target)
	}

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			itfStds := t.Engine.EnsureRecallCache(ctx, session, "interface_ast", "search", map[string]any{"namespace": "ecosystem",
				"query": "Go interface abstraction AST structural standards for " + input.Target,
				"limit": 10,
			})
			session.Metadata["recall_cache_interface"] = itfStds

			if itfStds != "" {
				summary += fmt.Sprintf("\n\n[Enterprise Architecture Standards]: %s", itfStds)
			}
		}

		// Pillar metrics for brainstorm learning.
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":                  "modularization",
			"interface_cluster_count": len(suggestions),
		}

		// AST faults.
		if len(suggestions) > 0 {
			var astFaults []string
			if f, ok := session.Metadata["ast_faults"].([]string); ok {
				astFaults = f
			}
			session.Metadata["ast_faults"] = append(astFaults, "interface_opportunity")
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "interfaces_discovered", "native", "go_interface_discovery", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string                `json:"summary"`
		Data    []interfaceSuggestion `json:"data"`
	}{
		Summary: summary,
		Data:    suggestions,
	}, nil
}

type ImplementationInput struct {
	models.UniversalPipelineInput
}

type ImplementationMatch struct {
	PkgPath string `json:"PkgPath"`
	Name    string `json:"Name"`
	File    string `json:"File"`
	Line    int    `json:"Line"`
}

func (t *ImplementationTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ImplementationInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", "find_interface_implementations")
	}

	if t.Engine != nil {
		sessionPkg := input.Target
		if sessionPkg == "" {
			sessionPkg = "./..."
		}
		session = t.Engine.LoadSession(ctx, sessionPkg)
	}

	if input.Target == "" {
		input.Target = "./..."
	}

	if input.Context == "" {
		summary := "Skipped interface search (no interface name provided)"
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
				t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "implementations_found", "native", "find_interface_implementations", "", session.Metadata)
			}
		}
		return &mcp.CallToolResult{}, struct {
			Summary string                `json:"summary"`
			Data    []ImplementationMatch `json:"data"`
		}{
			Summary: summary,
			Data:    []ImplementationMatch{},
		}, nil
	}

	matches, err := t.FindImplementations(ctx, req, input.Context, input.Target)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	summary := fmt.Sprintf("Found %d types implementing '%s' in %s", len(matches), input.Context, input.Target)

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		// Calculate downstream Blast Radius logic for safety thresholding
		distinctPkgs := make(map[string]bool)
		for _, m := range matches {
			distinctPkgs[m.PkgPath] = true
		}
		if len(matches) > 0 {
			session.Metadata["blast_radius"] = map[string]int{
				"distinct_downstream_packages": len(distinctPkgs),
				"direct_implementations":       len(matches),
			}
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		t.Engine.SaveSession(session)

		// Publish interface implementation trace to recall sessions matrix.
		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "implementations_found", "native", "find_interface_implementations", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string                `json:"summary"`
		Data    []ImplementationMatch `json:"data"`
	}{
		Summary: summary,
		Data:    matches,
	}, nil
}

func (t *ImplementationTool) FindImplementations(ctx context.Context, req *mcp.CallToolRequest, interfaceName, searchPkg string) ([]ImplementationMatch, error) {
	// 1. Resolve Target Interface
	pkgs, err := loader.LoadPackages(ctx, searchPkg, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	var targetInterface *types.Interface

	// Fast Path: Resolve local interface
	for _, p := range pkgs {
		obj := p.Types.Scope().Lookup(interfaceName)
		if obj != nil {
			if itf, ok := obj.Type().Underlying().(*types.Interface); ok {
				targetInterface = itf
				break
			}
		}
	}

	// Slow Path: Resolve qualified interface (e.g. io.Reader)
	if targetInterface == nil && strings.Contains(interfaceName, ".") {
		parts := strings.Split(interfaceName, ".")
		pkgPath := strings.Join(parts[:len(parts)-1], ".")
		typeName := parts[len(parts)-1]

		itfPkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
		if err == nil && len(itfPkgs) > 0 {
			obj := itfPkgs[0].Types.Scope().Lookup(typeName)
			if obj != nil {
				if itf, ok := obj.Type().Underlying().(*types.Interface); ok {
					targetInterface = itf
				}
			}
		}
	}

	if targetInterface == nil {
		return nil, fmt.Errorf("could not resolve interface '%s'", interfaceName)
	}

	// 2. Search for Implementations
	matches := []ImplementationMatch{}
	var mu sync.Mutex

	// Handle progress token
	var progressToken string
	if req.Params != nil {
		results := gjson.Get(string(req.Params.Arguments), "_meta.progressToken")
		if results.Exists() {
			progressToken = results.String()
		}
	}

	totalPkgs := len(pkgs)
	for i, pkg := range pkgs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Report Progress
		if progressToken != "" && req.Session != nil {
			_ = req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
				ProgressToken: progressToken,
				Progress:      float64(i + 1),
				Total:         float64(totalPkgs),
			})
		}

		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if obj == nil || !obj.Exported() {
				continue
			}

			typ := obj.Type()
			// Check both Type and *Type
			implementors := []types.Type{typ, types.NewPointer(typ)}
			for _, v := range implementors {
				if types.Implements(v, targetInterface) {
					pos := pkg.Fset.Position(obj.Pos())
					mu.Lock()
					matches = append(matches, ImplementationMatch{
						PkgPath: pkg.PkgPath,
						Name:    name,
						File:    pos.Filename,
						Line:    pos.Line,
					})
					mu.Unlock()
					break // Found match for this type (either value or pointer)
				}
			}
		}
	}

	return matches, nil
}

type interfaceSuggestion struct {
	Structs      []string              `json:"Structs"`
	Methods      []string              `json:"Methods"`
	Implementors []ImplementationMatch `json:"Implementors,omitempty"`
}

func DiscoverSharedInterfaces(ctx context.Context, pkgPath string) ([]interfaceSuggestion, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	var mu sync.Mutex
	allStructs := make(map[string][]*types.Func)
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
					methods := []*types.Func{}
					for method := range ms.Methods() {
						m := method.Obj().(*types.Func)
						if m.Exported() {
							methods = append(methods, m)
						}
					}
					if len(methods) > 0 {
						mu.Lock()
						// Store method pointers for accurate signature matching later
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

			intersect := signatureIntersection(commonMethods, methods2)
			if len(intersect) >= 2 {
				group = append(group, name2)
				commonMethods = intersect
				processed[name2] = true
			}
		}

		if len(group) > 1 {
			methodNames := []string{}
			for _, m := range commonMethods {
				methodNames = append(methodNames, m.Name())
			}
			suggestions = append(suggestions, interfaceSuggestion{
				Structs: group,
				Methods: methodNames,
			})
		}
	}

	// 3. Optional Global Implementation Scan for discovered patterns
	if len(suggestions) > 0 {
		allPkgs, err := loader.LoadPackages(ctx, "./...", loader.DefaultMode)
		if err == nil {
			for idx := range suggestions {
				sugg := &suggestions[idx]
				// Find common signatures again to build a synthetic interface
				// This is slightly redundant but cleaner.
				var commonSig []*types.Func
				for _, s := range sugg.Structs {
					if commonSig == nil {
						commonSig = allStructs[s]
					} else {
						commonSig = signatureIntersection(commonSig, allStructs[s])
					}
				}

				if len(commonSig) > 0 {
					itf := types.NewInterfaceType(commonSig, nil).Complete()
					for _, p := range allPkgs {
						scope := p.Types.Scope()
						for _, name := range scope.Names() {
							obj := scope.Lookup(name)
							if obj == nil || !obj.Exported() {
								continue
							}
							typ := obj.Type()
							implementors := []types.Type{typ, types.NewPointer(typ)}
							for _, v := range implementors {
								if types.Implements(v, itf) {
									pos := p.Fset.Position(obj.Pos())
									sugg.Implementors = append(sugg.Implementors, ImplementationMatch{
										PkgPath: p.PkgPath,
										Name:    name,
										File:    pos.Filename,
										Line:    pos.Line,
									})
									break
								}
							}
						}
					}
				}
			}
		}
	}

	return suggestions, nil
}

func signatureIntersection(s1, s2 []*types.Func) []*types.Func {
	res := []*types.Func{}
	for _, v1 := range s1 {
		for _, v2 := range s2 {
			if v1.Name() == v2.Name() && types.Identical(v1.Type(), v2.Type()) {
				res = append(res, v1)
				break
			}
		}
	}
	return res
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
