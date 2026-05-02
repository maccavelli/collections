package memory

import (
	"go/ast"

	"golang.org/x/tools/go/packages"
)

// DetectSyncPoolIssues analyzes packages for sync.Pool misuse patterns.
func DetectSyncPoolIssues(pkgs []*packages.Package) []Finding {
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
				detectPoolGetWithoutReset(pkg, fn.Body, funcName, &findings)
				return true
			})
		}
	}
	return findings
}

// detectPoolGetWithoutReset finds Pool.Get() calls that are used without
// resetting the returned object's state before use.
func detectPoolGetWithoutReset(pkg *packages.Package, body *ast.BlockStmt, funcName string, findings *[]Finding) {
	type poolGet struct {
		varName string
		pos     ast.Node
	}

	var gets []poolGet
	resetCalls := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			// Track x := pool.Get().
			if len(s.Rhs) != 1 || len(s.Lhs) < 1 {
				return true
			}
			call, ok := s.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			if isPoolGetCall(call) {
				if ident, ok := s.Lhs[0].(*ast.Ident); ok {
					gets = append(gets, poolGet{varName: ident.Name, pos: s})
				}
			}
		case *ast.ExprStmt:
			// Track x.Reset() calls.
			call, ok := s.X.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "Reset" {
				if ident, ok := sel.X.(*ast.Ident); ok {
					resetCalls[ident.Name] = true
				}
			}
		}
		return true
	})

	for _, g := range gets {
		if !resetCalls[g.varName] {
			pos := pkg.Fset.Position(g.pos.Pos())
			*findings = append(*findings, Finding{
				File:        pos.Filename,
				Line:        pos.Line,
				Function:    funcName,
				Category:    "sync_pool",
				Severity:    "MEDIUM",
				Pattern:     "pool_get_no_reset",
				Description: "sync.Pool object '" + g.varName + "' retrieved via Get() without calling Reset() — may contain stale data from previous use.",
				Suggestion:  "Call Reset() or manually zero the object's fields immediately after Get() before use.",
			})
		}
	}

	// Detect Pool.Put() patterns where the object might still be referenced.
	detectPoolPutWithReferences(pkg, body, funcName, findings)
}

// detectPoolPutWithReferences finds Put() calls where the pooled object is
// used after being returned to the pool.
func detectPoolPutWithReferences(pkg *packages.Package, body *ast.BlockStmt, funcName string, findings *[]Finding) {
	type putCall struct {
		varName string
		pos     ast.Node
		lineNum int
	}

	var puts []putCall
	usageAfterPut := make(map[string]bool)

	// First pass: find all pool.Put(x) calls and their positions.
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !isPoolPutCall(call) {
			return true
		}
		if len(call.Args) < 1 {
			return true
		}
		if ident, ok := call.Args[0].(*ast.Ident); ok {
			pos := pkg.Fset.Position(call.Pos())
			puts = append(puts, putCall{varName: ident.Name, pos: call, lineNum: pos.Line})
		}
		return true
	})

	if len(puts) == 0 {
		return
	}

	// Second pass: check for usage of the variable after the Put() call.
	for _, p := range puts {
		ast.Inspect(body, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok || ident.Name != p.varName {
				return true
			}
			pos := pkg.Fset.Position(ident.Pos())
			if pos.Line > p.lineNum {
				usageAfterPut[p.varName] = true
			}
			return true
		})
	}

	for _, p := range puts {
		if usageAfterPut[p.varName] {
			pos := pkg.Fset.Position(p.pos.Pos())
			*findings = append(*findings, Finding{
				File:        pos.Filename,
				Line:        pos.Line,
				Function:    funcName,
				Category:    "sync_pool",
				Severity:    "HIGH",
				Pattern:     "pool_put_use_after",
				Description: "Variable '" + p.varName + "' is used after being returned to sync.Pool via Put() — data race risk.",
				Suggestion:  "Ensure the variable is not referenced after Put(). Set the local variable to nil after Put().",
			})
		}
	}
}

// isPoolGetCall checks if a call expression is X.Get() on a sync.Pool-like object.
func isPoolGetCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "Get"
}

// isPoolPutCall checks if a call expression is X.Put() on a sync.Pool-like object.
func isPoolPutCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "Put"
}
