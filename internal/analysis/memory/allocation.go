package memory

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/packages"
)

// DetectAllocationIssues analyzes packages for unbounded allocation patterns.
func DetectAllocationIssues(pkgs []*packages.Package) []Finding {
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
				detectAppendInLoop(pkg, fn.Body, funcName, false, &findings)
				detectSprintfConcatenation(pkg, fn.Body, funcName, &findings)
				return true
			})
		}
	}
	return findings
}

// detectAppendInLoop finds append() calls inside loops where the target slice
// was not pre-allocated with make([]T, 0, cap).
func detectAppendInLoop(pkg *packages.Package, body *ast.BlockStmt, funcName string, inLoop bool, findings *[]Finding) {
	// Track slices declared with make([]T, 0, cap) — these are pre-allocated.
	preAllocated := make(map[string]bool)

	for _, stmt := range body.List {
		// Track pre-allocations: x := make([]T, 0, cap) or x = make([]T, 0, cap).
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if len(s.Lhs) >= 1 && len(s.Rhs) >= 1 {
				if ident, ok := s.Lhs[0].(*ast.Ident); ok {
					if isMakeSliceWithCap(s.Rhs[0]) {
						preAllocated[ident.Name] = true
					}
				}
			}
		case *ast.DeclStmt:
			if genDecl, ok := s.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
				for _, spec := range genDecl.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for i, val := range vs.Values {
							if isMakeSliceWithCap(val) && i < len(vs.Names) {
								preAllocated[vs.Names[i].Name] = true
							}
						}
					}
				}
			}
		}

		// Now recurse into loop bodies.
		switch s := stmt.(type) {
		case *ast.ForStmt:
			if s.Body != nil {
				detectAppendInLoopBody(pkg, s.Body, funcName, preAllocated, findings)
				detectAppendInLoop(pkg, s.Body, funcName, true, findings)
			}
		case *ast.RangeStmt:
			if s.Body != nil {
				detectAppendInLoopBody(pkg, s.Body, funcName, preAllocated, findings)
				detectAppendInLoop(pkg, s.Body, funcName, true, findings)
			}
		case *ast.IfStmt:
			if s.Body != nil {
				detectAppendInLoop(pkg, s.Body, funcName, inLoop, findings)
			}
		}
	}
}

// detectAppendInLoopBody finds append calls where the target was not pre-allocated.
func detectAppendInLoopBody(pkg *packages.Package, body *ast.BlockStmt, funcName string, preAllocated map[string]bool, findings *[]Finding) {
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if len(assign.Rhs) != 1 {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		fnIdent, ok := call.Fun.(*ast.Ident)
		if !ok || fnIdent.Name != "append" {
			return true
		}
		if len(call.Args) < 1 {
			return true
		}
		// Get the slice being appended to.
		sliceIdent, ok := call.Args[0].(*ast.Ident)
		if !ok {
			return true
		}
		if preAllocated[sliceIdent.Name] {
			return true
		}

		pos := pkg.Fset.Position(call.Pos())
		*findings = append(*findings, Finding{
			File:        pos.Filename,
			Line:        pos.Line,
			Function:    funcName,
			Category:    "allocation",
			Severity:    "MEDIUM",
			Pattern:     "append_in_loop_no_prealloc",
			Description: "append() called in a loop on slice '" + sliceIdent.Name + "' without pre-allocation — causes repeated memory reallocation and copying.",
			Suggestion:  "Pre-allocate with `" + sliceIdent.Name + " := make([]T, 0, expectedSize)` before the loop.",
		})
		return true
	})
}

// isMakeSliceWithCap checks if an expression is make([]T, len, cap) with a capacity argument.
func isMakeSliceWithCap(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name != "make" {
		return false
	}
	// make([]T, len, cap) — 3 args means capacity was specified.
	return len(call.Args) >= 3
}

// detectSprintfConcatenation finds fmt.Sprintf("%s%s", a, b) patterns that could
// use simpler concatenation or strings.Builder to avoid heap allocation.
func detectSprintfConcatenation(pkg *packages.Package, body *ast.BlockStmt, funcName string, findings *[]Finding) {
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
		if !ok || ident.Name != "fmt" || sel.Sel.Name != "Sprintf" {
			return true
		}
		if len(call.Args) < 2 {
			return true
		}

		// Check if the format string is purely "%s%s..." or "%v%v..." concatenation.
		formatLit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || formatLit.Kind != token.STRING {
			return true
		}
		format := formatLit.Value
		// Strip quotes.
		if len(format) >= 2 {
			format = format[1 : len(format)-1]
		}

		if isPureStringConcatFormat(format, len(call.Args)-1) {
			pos := pkg.Fset.Position(call.Pos())
			*findings = append(*findings, Finding{
				File:        pos.Filename,
				Line:        pos.Line,
				Function:    funcName,
				Category:    "allocation",
				Severity:    "LOW",
				Pattern:     "sprintf_simple_concat",
				Description: "fmt.Sprintf used for simple string concatenation — causes unnecessary heap allocation.",
				Suggestion:  "Use string concatenation (+) or strings.Builder for better performance.",
			})
		}
		return true
	})
}

// isPureStringConcatFormat checks if a format string is just %s/%v verbs with no other text.
func isPureStringConcatFormat(format string, argCount int) bool {
	if argCount < 2 {
		return false
	}
	// Remove all %s and %v verbs and check if anything remains.
	cleaned := format
	verbCount := 0
	for {
		idx := -1
		for i := range len(cleaned) - 1 {
			if cleaned[i] == '%' && (cleaned[i+1] == 's' || cleaned[i+1] == 'v') {
				idx = i
				break
			}
		}
		if idx == -1 {
			break
		}
		cleaned = cleaned[:idx] + cleaned[idx+2:]
		verbCount++
	}
	return cleaned == "" && verbCount == argCount
}
