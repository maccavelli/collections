// Package memory provides functionality for the memory subsystem.
package memory

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// Finding represents a single memory-related diagnostic finding.
type Finding struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Function    string `json:"function"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Pattern     string `json:"pattern"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
	ASTTrace    string `json:"ast_trace,omitempty"`
}

// DetectGoroutineLeaks analyzes packages for goroutine leak patterns.
func DetectGoroutineLeaks(pkgs []*packages.Package) []Finding {
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

				inspectBody(pkg, file, fn.Body, funcName, false, &findings)
				return true
			})
		}
	}
	return findings
}

// inspectBody walks a block statement looking for goroutine leak patterns.
func inspectBody(pkg *packages.Package, file *ast.File, body *ast.BlockStmt, funcName string, inLoop bool, findings *[]Finding) {
	for _, stmt := range body.List {
		switch s := stmt.(type) {
		case *ast.GoStmt:
			checkGoroutineLeak(pkg, s, funcName, inLoop, findings)
		case *ast.ForStmt:
			if s.Body != nil {
				inspectBody(pkg, file, s.Body, funcName, true, findings)
			}
		case *ast.RangeStmt:
			if s.Body != nil {
				inspectBody(pkg, file, s.Body, funcName, true, findings)
			}
		case *ast.IfStmt:
			if s.Body != nil {
				inspectBody(pkg, file, s.Body, funcName, inLoop, findings)
			}
			if s.Else != nil {
				if block, ok := s.Else.(*ast.BlockStmt); ok {
					inspectBody(pkg, file, block, funcName, inLoop, findings)
				}
			}
		case *ast.SelectStmt:
			if s.Body != nil {
				inspectBody(pkg, file, s.Body, funcName, inLoop, findings)
			}
		case *ast.SwitchStmt:
			if s.Body != nil {
				inspectBody(pkg, file, s.Body, funcName, inLoop, findings)
			}
		case *ast.CaseClause:
			for _, inner := range s.Body {
				if block, ok := inner.(*ast.BlockStmt); ok {
					inspectBody(pkg, file, block, funcName, inLoop, findings)
				}
			}
		}

		// Detect time.After in loops.
		if inLoop {
			ast.Inspect(stmt, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if isTimeAfterCall(call) {
					pos := pkg.Fset.Position(call.Pos())
					*findings = append(*findings, Finding{
						File:        pos.Filename,
						Line:        pos.Line,
						Function:    funcName,
						Category:    "goroutine_leak",
						Severity:    "HIGH",
						Pattern:     "time_after_in_loop",
						Description: "time.After called inside a loop — creates a new timer per iteration. Old timers leak until they fire.",
						Suggestion:  "Use time.NewTimer with Reset(), or use a select with a single timer created outside the loop.",
					})
				}
				return true
			})
		}
	}
}

// checkGoroutineLeak evaluates a go statement for leak patterns.
func checkGoroutineLeak(pkg *packages.Package, goStmt *ast.GoStmt, funcName string, inLoop bool, findings *[]Finding) {
	pos := pkg.Fset.Position(goStmt.Pos())

	// Pattern: goroutine launched inside a loop.
	if inLoop {
		*findings = append(*findings, Finding{
			File:        pos.Filename,
			Line:        pos.Line,
			Function:    funcName,
			Category:    "goroutine_leak",
			Severity:    "CRITICAL",
			Pattern:     "goroutine_in_loop",
			Description: "Goroutine launched inside a loop — unbounded goroutine spawning can exhaust memory.",
			Suggestion:  "Use a worker pool pattern (e.g., bounded semaphore channel) or errgroup.Group to cap concurrency.",
		})
	}

	// Pattern: goroutine without context parameter.
	funcLit, ok := goStmt.Call.Fun.(*ast.FuncLit)
	if !ok {
		return
	}

	if !funcLitAcceptsContext(funcLit) && !funcLitCapturesContext(funcLit) {
		*findings = append(*findings, Finding{
			File:        pos.Filename,
			Line:        pos.Line,
			Function:    funcName,
			Category:    "goroutine_leak",
			Severity:    "HIGH",
			Pattern:     "goroutine_no_context",
			Description: "Goroutine launched without context.Context — no cancellation path available.",
			Suggestion:  "Pass a context.Context to the goroutine and select on ctx.Done() to enable cancellation.",
		})
	}

	// Pattern: channel operations without select + ctx.Done.
	if funcLit.Body != nil {
		hasChannelOp := false
		hasSelectWithDone := false

		ast.Inspect(funcLit.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SendStmt:
				hasChannelOp = true
			case *ast.UnaryExpr:
				if x.Op.String() == "<-" {
					hasChannelOp = true
				}
			case *ast.SelectStmt:
				if selectHasCtxDone(x) {
					hasSelectWithDone = true
				}
			}
			return true
		})

		if hasChannelOp && !hasSelectWithDone {
			*findings = append(*findings, Finding{
				File:        pos.Filename,
				Line:        pos.Line,
				Function:    funcName,
				Category:    "goroutine_leak",
				Severity:    "MEDIUM",
				Pattern:     "channel_without_select_done",
				Description: "Goroutine performs channel operations without select + ctx.Done() — may block indefinitely.",
				Suggestion:  "Wrap channel operations in a select with a case <-ctx.Done() for cancellation.",
			})
		}
	}
}

// funcLitAcceptsContext checks if a function literal has a context.Context parameter.
func funcLitAcceptsContext(fn *ast.FuncLit) bool {
	if fn.Type == nil || fn.Type.Params == nil {
		return false
	}
	for _, field := range fn.Type.Params.List {
		if isContextType(field.Type) {
			return true
		}
	}
	return false
}

// funcLitCapturesContext checks if a function literal references a variable named "ctx" from its closure.
func funcLitCapturesContext(fn *ast.FuncLit) bool {
	if fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		ident, ok := n.(*ast.Ident)
		if ok && ident.Name == "ctx" {
			found = true
		}
		return true
	})
	return found
}

// selectHasCtxDone checks if a select statement has a case <-ctx.Done().
func selectHasCtxDone(sel *ast.SelectStmt) bool {
	if sel.Body == nil {
		return false
	}
	for _, stmt := range sel.Body.List {
		cc, ok := stmt.(*ast.CommClause)
		if !ok || cc.Comm == nil {
			continue
		}
		// Check for <-ctx.Done() in either ExprStmt or AssignStmt forms.
		var expr ast.Expr
		switch s := cc.Comm.(type) {
		case *ast.ExprStmt:
			if ue, ok := s.X.(*ast.UnaryExpr); ok {
				expr = ue.X
			}
		case *ast.AssignStmt:
			if len(s.Rhs) == 1 {
				if ue, ok := s.Rhs[0].(*ast.UnaryExpr); ok {
					expr = ue.X
				}
			}
		}
		if expr == nil {
			continue
		}
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		if sel.Sel.Name == "Done" {
			return true
		}
	}
	return false
}

// isContextType checks if an expression is context.Context.
func isContextType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name != "Context" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "context"
}

// isTimeAfterCall checks if a call expression is time.After().
func isTimeAfterCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name != "After" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "time"
}

// isWaitGroupType checks if a types.Object is a sync.WaitGroup.
func isWaitGroupType(obj types.Object) bool {
	if obj == nil {
		return false
	}
	return obj.Type().String() == "sync.WaitGroup" || obj.Type().String() == "*sync.WaitGroup"
}
