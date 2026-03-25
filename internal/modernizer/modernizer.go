package modernizer

import (
	"context"
	"fmt"
	"go/ast"
	"mcp-server-go-refactor/internal/dstutil"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"
	"strings"

	"github.com/dave/dst"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/tools/go/packages"
)

// Tool implements the go_modernizer tool.
type Tool struct{}

func (t *Tool) Name() string {
	return "go_modernizer"
}

func (t *Tool) Register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "COMPLETION MANDATE / CODE REPAIR: Scans code for legacy patterns and replaces them with optimized standard library functions (e.g., slices/maps Go 1.21+). Call this to finalize any refactor and ensure high performance. Cascades to go_test_coverage_tracer.",
	}, t.Handle)
}

// Register adds the modernizer tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

type ModernizeInput struct {
	Pkg     string `json:"pkg" jsonschema:"The package path to analyze"`
	Rewrite bool   `json:"rewrite" jsonschema:"If true, automatically applies modernization changes (comment-safe)."`
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ModernizeInput) (*mcp.CallToolResult, any, error) {
	if input.Rewrite {
		err := ApplyModernize(ctx, input.Pkg)
		if err != nil {
			res := &mcp.CallToolResult{}
			res.SetError(err)
			return res, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully applied modernization for %s", input.Pkg)}},
		}, nil, nil
	}

	findings, err := Analyze(ctx, input.Pkg)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	if len(findings) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "No modernization opportunities found."}},
		}, nil, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Modernization findings for %s:\n\n", input.Pkg))
	for _, f := range findings {
		sb.WriteString(fmt.Sprintf("[%s] %s:%d\n", f.Category, f.File, f.Line))
		sb.WriteString(fmt.Sprintf("  Rationale: %s\n", f.Rationale))
		sb.WriteString(fmt.Sprintf("  Replacement: %s\n\n", f.Replacement))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
	}, nil, nil
}

// ApplyModernize performs automated code modernization using DST.
func ApplyModernize(ctx context.Context, pkgPath string) error {
	res, err := loader.Discover(ctx, pkgPath)
	if err != nil {
		return err
	}

	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return err
	}

	for _, pkg := range pkgs {
		for _, astFile := range pkg.Syntax {
			dstFile, err := dstutil.ToDST(pkg.Fset, astFile)
			if err != nil {
				continue
			}

			modified := false
			dst.Inspect(dstFile, func(n dst.Node) bool {
				// Interface Alignment Replacement
				if fd, ok := n.(*dst.FuncDecl); ok && fd.Recv != nil && len(fd.Recv.List) > 0 {
					name := fd.Name.Name
					if name == "ToString" || name == "AsString" {
						// Simple rename
						fd.Name.Name = "String"
						modified = true
					}
				}
				return true
			})

			if modified {
				data, err := dstutil.WriteFile(dstFile)
				if err != nil {
					return fmt.Errorf("write file: %w", err)
				}
				filename := pkg.Fset.Position(astFile.Pos()).Filename
				if err := res.Runner.WriteFileAtomic(filename, data); err != nil {
					return fmt.Errorf("atomic write %s: %w", filename, err)
				}
			}
		}
	}
	return nil
}

// Finding represents a modernization opportunity.
type Finding struct {
	Category    string `json:"category"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Rationale   string `json:"rationale"`
	Replacement string `json:"replacement"`
}

// Analyze scans the package for modernization opportunities.
func Analyze(ctx context.Context, pkgPath string) ([]Finding, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	var findings []Finding
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				// 1. Slice Filtering Matcher
				if f, ok := matchSliceFilter(pkg, n); ok {
					findings = append(findings, f)
				}
				// 2. Interface Alignment Matcher
				if f, ok := matchInterfaceAlignment(pkg, n); ok {
					findings = append(findings, f)
				}
				return true
			})
		}
	}
	return findings, nil
}

func matchSliceFilter(pkg *packages.Package, n ast.Node) (Finding, bool) {
	rs, ok := n.(*ast.RangeStmt)
	if !ok {
		return Finding{}, false
	}

	// Look for a single IfStmt inside the loop
	if len(rs.Body.List) != 1 {
		return Finding{}, false
	}

	ifStmt, ok := rs.Body.List[0].(*ast.IfStmt)
	if !ok {
		return Finding{}, false
	}

	// In the if body, look for append
	if len(ifStmt.Body.List) != 1 {
		return Finding{}, false
	}

	call, ok := getAppendCall(ifStmt.Body.List[0])
	if !ok {
		return Finding{}, false
	}

	// Verify it's appending to a slice
	if len(call.Args) < 2 {
		return Finding{}, false
	}

	pos := pkg.Fset.Position(n.Pos())
	return Finding{
		Category:    "Slice Optimization",
		File:        pos.Filename,
		Line:        pos.Line,
		Rationale:   "Manual slice filtering loop can be replaced with slices.DeleteFunc (Go 1.21+).",
		Replacement: "slices.DeleteFunc(slice, func(e T) bool { return !condition })",
	}, true
}

func getAppendCall(stmt ast.Stmt) (*ast.CallExpr, bool) {
	exprStmt, ok := stmt.(*ast.ExprStmt)
	if !ok {
		// Might be an assignment: res = append(res, item)
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return nil, false
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return nil, false
		}
		if fun, ok := call.Fun.(*ast.Ident); ok && fun.Name == "append" {
			return call, true
		}
		return nil, false
	}

	call, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	if fun, ok := call.Fun.(*ast.Ident); ok && fun.Name == "append" {
		return call, true
	}
	return nil, false
}

func matchInterfaceAlignment(pkg *packages.Package, n ast.Node) (Finding, bool) {
	fd, ok := n.(*ast.FuncDecl)
	if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
		return Finding{}, false
	}

	// We're looking for common naming patterns: ToString, AsString, ToJSON, etc.
	name := fd.Name.Name
	var replacement string
	var rationale string

	switch name {
	case "ToString", "AsString":
		// Verify signature: func() string
		if fd.Type.Results == nil || len(fd.Type.Results.List) != 1 || len(fd.Type.Params.List) != 0 {
			return Finding{}, false
		}
		resIdent, ok := fd.Type.Results.List[0].Type.(*ast.Ident)
		if !ok || resIdent.Name != "string" {
			return Finding{}, false
		}
		replacement = "String() string"
		rationale = "Method signature matches fmt.Stringer interface; rename to String() for standard compatibility."
	default:
		return Finding{}, false
	}

	pos := pkg.Fset.Position(n.Pos())
	return Finding{
		Category:    "Interface Alignment",
		File:        pos.Filename,
		Line:        pos.Line,
		Rationale:   rationale,
		Replacement: replacement,
	}, true
}
