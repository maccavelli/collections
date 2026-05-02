package astutil

import (
	"context"
	"fmt"
	"go/types"
	"log/slog"
	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/tools/go/packages"
)

// Tool implements the interface analyzer tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_interface_tool"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: MUTATOR] INTERFACE EXTRACTOR: Makes, creates, and extracts formal interface definitions from structs or verifies compatibility. Performs AST-driven interface generation. Use AFTER go_interface_discovery confirms it is safe. Produces extracted interface definition or compatibility verdict. In orchestrator mode, publishes telemetry.",
	}, t.Handle)
}

// Register adds the interface analytical tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

type InterfaceInput struct {
	models.UniversalPipelineInput
	Pkg        string  `json:"pkg" jsonschema:"The package path"`
	StructName string  `json:"structName,omitempty" jsonschema:"The name of the struct (optional). If omitted, skips interface analysis."`
	IfaceName  *string `json:"ifaceName,omitempty" jsonschema:"The name of the interface to check against (optional). If omitted, extracts a new interface from the struct."`
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input InterfaceInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Pkg)
	}

	// Graceful early-return when upstream discovery found no structs.
	if input.StructName == "" {
		summary := "Skipped interface analysis (no struct name provided by upstream stage)"
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
				t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Pkg, "interface_skipped", "native", "go_interface_tool", "", session.Metadata)
			}
		}
		return &mcp.CallToolResult{}, struct {
			Summary string `json:"summary"`
		}{
			Summary: summary,
		}, nil
	}

	if input.IfaceName != nil && *input.IfaceName != "" {
		result, err := AnalyzeInterface(ctx, input.Pkg, input.StructName, *input.IfaceName)
		if err != nil {
			res := &mcp.CallToolResult{}
			res.SetError(err)
			return res, nil, nil
		}
		summary := fmt.Sprintf("Interface compatibility check for %s vs %s: %v", input.StructName, *input.IfaceName, result.IsCompatible)

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

			// Publish interface analysis trace to recall sessions matrix.
			if recallAvailable {
				t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Pkg, "interface_refactored", "native", "go_interface_tool", "", session.Metadata)
			}
		}

		return &mcp.CallToolResult{}, struct {
			Summary string    `json:"summary"`
			Data    *Analysis `json:"data"`
		}{
			Summary: summary,
			Data:    result,
		}, nil
	}

	result, err := ExtractInterface(ctx, input.Pkg, input.StructName)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	summary := fmt.Sprintf("Extracted interface %s from struct %s", result.InterfaceName, input.StructName)

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

		// Publish interface analysis trace to recall sessions matrix.
		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Pkg, "interface_refactored", "native", "go_interface_tool", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string            `json:"summary"`
		Data    *ExtractionResult `json:"data"`
	}{
		Summary: summary,
		Data:    result,
	}, nil
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

	for m := range iface.Methods() {
		m := m
		// We use LookupFieldOrMethod to check for existence; index and indirect are implicitly ignored.
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
	for method := range ms.Methods() {
		m := method.Obj().(*types.Func)
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
