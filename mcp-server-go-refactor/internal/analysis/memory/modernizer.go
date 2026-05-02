package memory

import (
	"go/ast"

	"golang.org/x/tools/go/packages"
)

// DetectModernPatterns analyzes packages for Go 1.26-specific memory patterns.
func DetectModernPatterns(pkgs []*packages.Package) []Finding {
	var findings []Finding

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				fn, ok := n.(*ast.FuncDecl)
				if !ok {
					return true
				}
				if fn.Body == nil {
					return true
				}

				funcName := fn.Name.Name
				detectManualGC(pkg, fn.Body, funcName, &findings)
				detectMissingKeepAlive(pkg, fn.Body, funcName, &findings)
				detectManualZeroLoop(pkg, fn.Body, funcName, &findings)
				return true
			})
		}
	}
	return findings
}

// detectManualGC finds explicit runtime.GC() calls.
// With Go 1.26's Green Tea GC, manual GC invocations are rarely needed.
func detectManualGC(pkg *packages.Package, body *ast.BlockStmt, funcName string, findings *[]Finding) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name == "runtime" && sel.Sel.Name == "GC" {
			pos := pkg.Fset.Position(call.Pos())
			*findings = append(*findings, Finding{
				File:        pos.Filename,
				Line:        pos.Line,
				Function:    funcName,
				Category:    "modern_patterns",
				Severity:    "WARNING",
				Pattern:     "manual_gc_call",
				Description: "Explicit runtime.GC() call — Go 1.26's Green Tea GC is highly optimized and rarely needs manual triggering.",
				Suggestion:  "Remove unless benchmarks prove this specific GC call reduces latency. Green Tea GC provides 10-40% lower overhead automatically.",
			})
		}
		return true
	})
}

// detectMissingKeepAlive finds runtime.SetFinalizer calls without corresponding
// runtime.KeepAlive in the same function scope.
func detectMissingKeepAlive(pkg *packages.Package, body *ast.BlockStmt, funcName string, findings *[]Finding) {
	hasSetFinalizer := false
	hasKeepAlive := false
	var finalizerPos ast.Node

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name == "runtime" {
			if sel.Sel.Name == "SetFinalizer" {
				hasSetFinalizer = true
				finalizerPos = call
			}
			if sel.Sel.Name == "KeepAlive" {
				hasKeepAlive = true
			}
		}
		return true
	})

	if hasSetFinalizer && !hasKeepAlive && finalizerPos != nil {
		pos := pkg.Fset.Position(finalizerPos.Pos())
		*findings = append(*findings, Finding{
			File:        pos.Filename,
			Line:        pos.Line,
			Function:    funcName,
			Category:    "modern_patterns",
			Severity:    "HIGH",
			Pattern:     "finalizer_no_keepalive",
			Description: "runtime.SetFinalizer used without runtime.KeepAlive — object may be collected before finalizer runs.",
			Suggestion:  "Add runtime.KeepAlive(obj) at the end of the scope where the finalized object must remain alive.",
		})
	}
}

// detectManualZeroLoop finds manual zeroing loops that could use the clear() builtin.
// Pattern: for i := range s { s[i] = zero }
func detectManualZeroLoop(pkg *packages.Package, body *ast.BlockStmt, funcName string, findings *[]Finding) {
	for _, stmt := range body.List {
		rangeStmt, ok := stmt.(*ast.RangeStmt)
		if !ok {
			continue
		}
		if rangeStmt.Body == nil || len(rangeStmt.Body.List) != 1 {
			continue
		}

		// Check for single assignment: s[i] = zeroValue.
		assign, ok := rangeStmt.Body.List[0].(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			continue
		}

		// LHS must be an index expression: s[i].
		indexExpr, ok := assign.Lhs[0].(*ast.IndexExpr)
		if !ok {
			continue
		}

		// Verify the loop variable matches the index.
		if rangeStmt.Key == nil {
			continue
		}
		keyIdent, ok := rangeStmt.Key.(*ast.Ident)
		if !ok {
			continue
		}
		indexIdent, ok := indexExpr.Index.(*ast.Ident)
		if !ok || indexIdent.Name != keyIdent.Name {
			continue
		}

		// Verify the collection being ranged matches the indexed collection.
		rangeIdent, ok := rangeStmt.X.(*ast.Ident)
		if !ok {
			continue
		}
		collIdent, ok := indexExpr.X.(*ast.Ident)
		if !ok || collIdent.Name != rangeIdent.Name {
			continue
		}

		// Check if RHS is a zero value (literal 0, "", false, nil, or composite {}).
		if isZeroValue(assign.Rhs[0]) {
			pos := pkg.Fset.Position(rangeStmt.Pos())
			*findings = append(*findings, Finding{
				File:        pos.Filename,
				Line:        pos.Line,
				Function:    funcName,
				Category:    "modern_patterns",
				Severity:    "LOW",
				Pattern:     "manual_zero_loop",
				Description: "Manual zeroing loop detected — can be replaced with the clear() builtin (available since Go 1.21).",
				Suggestion:  "Replace with `clear(" + rangeIdent.Name + ")` for cleaner, potentially faster zeroing.",
			})
		}
	}
}

// isZeroValue checks if an expression is a zero-value literal.
func isZeroValue(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.BasicLit:
		return x.Value == "0" || x.Value == `""` || x.Value == "``" || x.Value == "0.0"
	case *ast.Ident:
		return x.Name == "false" || x.Name == "nil"
	case *ast.CompositeLit:
		return len(x.Elts) == 0
	}
	return false
}
