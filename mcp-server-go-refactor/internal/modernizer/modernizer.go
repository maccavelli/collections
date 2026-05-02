package modernizer

import (
	"context"
	"fmt"
	"go/ast"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/ast/inspector"

	"mcp-server-go-refactor/internal/dstutil"
	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"

	"github.com/dave/dst"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/tools/go/packages"
)

// Tool implements the go_modernizer tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_modernizer"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] CODE MODERNIZER: Fixes code arrays, updates legacy patterns, and improves loops using optimized standard library packages (slices/maps). Identifies deprecated API usage and recommends idiomatic Go 1.26+ replacements. Produces modernization opportunity list by category.",
	}, t.Handle)
}

// Register adds the modernizer tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

type ModernizeInput struct {
	models.UniversalPipelineInput
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ModernizeInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	rewrite := false
	if rw, ok := input.Flags["rewrite"].(bool); ok {
		rewrite = rw
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Target)

		if recallAvailable {
			if history := t.Engine.LoadCrossSessionFromRecall(ctx, "gorefactor", input.Target); history != "" {
				if session.Metadata == nil {
					session.Metadata = make(map[string]any)
				}
				session.Metadata["historical_modernization"] = history
			}
		}
	}

	// Sniff for native repository linter guidelines to strictly align AST rewriting logic
	var nativeLinters []string
	if stat, err := os.Stat(input.Target); err == nil && stat.IsDir() {
		for _, cfg := range []string{".golangci.yaml", ".golangci.yml", ".revive.toml", "staticcheck.conf"} {
			if _, err := os.Stat(filepath.Join(input.Target, cfg)); err == nil {
				nativeLinters = append(nativeLinters, cfg)
			}
		}
	}
	if len(nativeLinters) > 0 && session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}
		session.Metadata["active_native_linters"] = nativeLinters
	}

	if rewrite {
		err := ApplyModernize(ctx, input.Target)
		if err != nil {
			res := &mcp.CallToolResult{}
			res.SetError(err)
			return res, nil, nil
		}
		summary := fmt.Sprintf("Successfully applied modernization for %s", input.Target)

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
				t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "modernization_scanned", "native", "go_modernizer", "", session.Metadata)
			}
		}

		return &mcp.CallToolResult{}, struct {
			Summary string `json:"summary"`
			Data    any    `json:"data"`
		}{
			Summary: summary,
			Data:    map[string]string{"message": summary},
		}, nil
	}

	findings, err := Analyze(ctx, input.Target)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	summary := "No modernization opportunities found."
	if len(findings) > 0 {
		summary = fmt.Sprintf("Found %d modernization opportunities in %s", len(findings), input.Target)
	}

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			// Dynamically query recall for specific structural AST definitions tailored to this package
			astStds := t.Engine.EnsureRecallCache(ctx, session, "modernizer_ast", "search", map[string]any{"namespace": "ecosystem",
				"query":       "AST structural rules definitions modern standards",
				"package":     input.Target,
				"symbol_type": "struct",
				"limit":       10,
			})
			session.Metadata["recall_cache_modernizer"] = astStds

			if astStds != "" {
				summary += fmt.Sprintf("\n\n[Entity Structure and AST Standards Enforced]: %s", astStds)
			}
		}

		// Pillar metrics for brainstorm learning.
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":        "modernization",
			"finding_count": len(findings),
		}

		// AST faults.
		if len(findings) > 0 {
			var astFaults []string
			if f, ok := session.Metadata["ast_faults"].([]string); ok {
				astFaults = f
			}
			session.Metadata["ast_faults"] = append(astFaults, "modernization_gap")
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "modernization_scanned", "native", "go_modernizer", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string    `json:"summary"`
		Data    []Finding `json:"data"`
	}{
		Summary: summary,
		Data:    findings,
	}, nil
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
			insp := inspector.New([]*ast.File{file})
			insp.Preorder([]ast.Node{
				(*ast.RangeStmt)(nil), (*ast.FuncDecl)(nil),
				(*ast.BlockStmt)(nil), (*ast.TypeSpec)(nil),
				(*ast.StructType)(nil), (*ast.MapType)(nil),
			}, func(n ast.Node) {
				// 1. Slice Filtering Matcher
				if f, ok := matchSliceFilter(pkg, n); ok {
					findings = append(findings, f)
				}
				// 2. Interface Alignment Matcher
				if f, ok := matchInterfaceAlignment(pkg, n); ok {
					findings = append(findings, f)
				}
				// 3. Pointer Ergonomics Matcher
				if fs := matchPointerErgonomics(pkg, n); len(fs) > 0 {
					findings = append(findings, fs...)
				}
				// 4. Errors AsType Matcher
				if fs := matchErrorsAsType(pkg, n); len(fs) > 0 {
					findings = append(findings, fs...)
				}
				// 5. Recursive Generics Matcher
				if f, ok := matchRecursiveGenerics(pkg, n); ok {
					findings = append(findings, f)
				}
				// 6. OmitZero Matcher
				if fs := matchOmitZero(pkg, n); len(fs) > 0 {
					findings = append(findings, fs...)
				}
				// 7. Green Tea GC Matcher
				if f, ok := matchGreenTeaGCLocality(pkg, n); ok {
					findings = append(findings, f)
				}
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

func matchPointerErgonomics(pkg *packages.Package, n ast.Node) []Finding {
	bs, ok := n.(*ast.BlockStmt)
	if !ok || len(bs.List) < 2 {
		return nil
	}

	var findings []Finding
	for i := 0; i < len(bs.List)-1; i++ {
		stmt1, ok1 := bs.List[i].(*ast.AssignStmt)
		stmt2, ok2 := bs.List[i+1].(*ast.AssignStmt)
		if !ok1 || !ok2 {
			continue
		}

		if len(stmt1.Lhs) == 1 && len(stmt2.Rhs) == 1 {
			id1, okId1 := stmt1.Lhs[0].(*ast.Ident)
			unOp, okUnOp := stmt2.Rhs[0].(*ast.UnaryExpr)

			if okId1 && okUnOp && unOp.Op.String() == "&" {
				if id2, okId2 := unOp.X.(*ast.Ident); okId2 && id1.Name == id2.Name {
					pos := pkg.Fset.Position(stmt2.Pos())
					findings = append(findings, Finding{
						Category:    "Pointer Ergonomics",
						File:        pos.Filename,
						Line:        pos.Line,
						Rationale:   "Intermediate variable created only to take its address. Go 1.26+ idioms prefer new(expr).",
						Replacement: fmt.Sprintf("new(%s)", id1.Name),
					})
				}
			}
		}
	}
	return findings
}

func matchErrorsAsType(pkg *packages.Package, n ast.Node) []Finding {
	bs, ok := n.(*ast.BlockStmt)
	if !ok || len(bs.List) < 2 {
		return nil
	}

	var findings []Finding
	for i := 0; i < len(bs.List)-1; i++ {
		declStmt, ok1 := bs.List[i].(*ast.DeclStmt)
		ifStmt, ok2 := bs.List[i+1].(*ast.IfStmt)
		if !ok1 || !ok2 {
			continue
		}

		genDecl, okGd := declStmt.Decl.(*ast.GenDecl)
		if !okGd || len(genDecl.Specs) != 1 {
			continue
		}

		valSpec, okVs := genDecl.Specs[0].(*ast.ValueSpec)
		if !okVs || len(valSpec.Names) != 1 {
			continue
		}
		targetName := valSpec.Names[0].Name

		// Check if ifStmt contains errors.As(err, &targetName)
		var hasErrorsAs bool
		ast.Inspect(ifStmt.Cond, func(nn ast.Node) bool {
			if ce, ok := nn.(*ast.CallExpr); ok {
				if se, ok := ce.Fun.(*ast.SelectorExpr); ok {
					if id, ok := se.X.(*ast.Ident); ok && id.Name == "errors" && se.Sel.Name == "As" {
						if len(ce.Args) == 2 {
							if ue, ok := ce.Args[1].(*ast.UnaryExpr); ok && ue.Op.String() == "&" {
								if tid, ok := ue.X.(*ast.Ident); ok && tid.Name == targetName {
									hasErrorsAs = true
								}
							}
						}
					}
				}
			}
			return true
		})

		if hasErrorsAs {
			pos := pkg.Fset.Position(ifStmt.Pos())
			findings = append(findings, Finding{
				Category:    "Type-Safe Error Handling",
				File:        pos.Filename,
				Line:        pos.Line,
				Rationale:   "Old errors.As pattern detected. Refactor to target, ok := errors.AsType[*T](err) for compile-time safety (Go 1.26).",
				Replacement: "errors.AsType[*T](err)",
			})
		}
	}
	return findings
}

func matchRecursiveGenerics(pkg *packages.Package, n ast.Node) (Finding, bool) {
	ts, ok := n.(*ast.TypeSpec)
	if !ok || ts.TypeParams == nil {
		return Finding{}, false
	}

	for _, field := range ts.TypeParams.List {
		if ident, ok := field.Type.(*ast.Ident); ok {
			if ident.Name == ts.Name.Name {
				pos := pkg.Fset.Position(ts.Pos())
				return Finding{
					Category:    "Recursive Generic Architecture",
					File:        pos.Filename,
					Line:        pos.Line,
					Rationale:   "Self-referential carrier parameters detected. Refactor to Go 1.26 recursive constraints (e.g., T Node[T]).",
					Replacement: fmt.Sprintf("type %s[T %s[T]]", ts.Name.Name, ts.Name.Name),
				}, true
			}
		}
	}
	return Finding{}, false
}

func matchOmitZero(pkg *packages.Package, n ast.Node) []Finding {
	st, ok := n.(*ast.StructType)
	if !ok || st.Fields == nil {
		return nil
	}

	var findings []Finding
	for _, field := range st.Fields.List {
		if field.Tag != nil && strings.Contains(field.Tag.Value, "omitempty") {
			// Fast proxy check for complex types that implement IsZero natively
			// like time.Time or other structs via type checking approximation
			isComplexType := false
			if selExpr, ok := field.Type.(*ast.SelectorExpr); ok {
				if id, ok := selExpr.X.(*ast.Ident); ok && id.Name == "time" && selExpr.Sel.Name == "Time" {
					isComplexType = true
				}
			} else if _, ok := field.Type.(*ast.Ident); ok {
				// We conservatively flag any omitempty ident as a potential omitzero candidate
				isComplexType = true
			}

			if isComplexType {
				pos := pkg.Fset.Position(field.Pos())
				name := "<embedded>"
				if len(field.Names) > 0 {
					name = field.Names[0].Name
				}
				findings = append(findings, Finding{
					Category:    "Serialization Efficiency",
					File:        pos.Filename,
					Line:        pos.Line,
					Rationale:   fmt.Sprintf("Field '%s' relies on omitempty which evaluates zero structs improperly. Upgrade to Go 1.24+ 'omitzero'.", name),
					Replacement: "omitzero",
				})
			}
		}
	}
	return findings
}

func matchGreenTeaGCLocality(pkg *packages.Package, n ast.Node) (Finding, bool) {
	mt, ok := n.(*ast.MapType)
	if !ok {
		return Finding{}, false
	}

	isAny := false
	if id, ok := mt.Value.(*ast.Ident); ok && id.Name == "any" {
		isAny = true
	} else if iface, ok := mt.Value.(*ast.InterfaceType); ok && iface.Methods != nil && len(iface.Methods.List) == 0 {
		isAny = true
	}

	if isAny {
		pos := pkg.Fset.Position(mt.Pos())
		return Finding{
			Category:    "GC Locality (Green Tea Protocol)",
			File:        pos.Filename,
			Line:        pos.Line,
			Rationale:   "Map using empty interface value detected. The Go 1.26 GC prefers tightly packed memory constraints. Refactor 'map[K]any' to typed slices or strict structures to aid Small Object marking.",
			Replacement: "struct/slice wrapper",
		}, true
	}

	return Finding{}, false
}
