package memory

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/packages"
)

// resourceOpener defines patterns for functions that open resources requiring Close().
type resourceOpener struct {
	PkgName  string   // e.g., "os", "net", "sql", "http"
	FuncName string   // e.g., "Open", "Create", "Dial"
	Aliases  []string // alternative selectors (e.g., "OpenFile")
}

var knownOpeners = []resourceOpener{
	{PkgName: "os", FuncName: "Open", Aliases: []string{"OpenFile", "Create", "CreateTemp"}},
	{PkgName: "net", FuncName: "Dial", Aliases: []string{"DialTimeout", "Listen", "ListenPacket"}},
	{PkgName: "sql", FuncName: "Open"},
	{PkgName: "tls", FuncName: "Dial", Aliases: []string{"DialWithDialer"}},
}

// httpResponsePatterns lists HTTP methods whose responses must have Body.Close().
var httpResponseMethods = []string{"Get", "Post", "Head", "Do", "PostForm"}

// DetectResourceLeaks analyzes packages for missing defer Close() calls.
func DetectResourceLeaks(pkgs []*packages.Package) []Finding {
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
				analyzeResourceLeaks(pkg, fn.Body, funcName, &findings)
				detectDeferInLoop(pkg, fn.Body, funcName, false, &findings)
				return true
			})
		}
	}
	return findings
}

// analyzeResourceLeaks scans a function body for open calls without matching defer close.
func analyzeResourceLeaks(pkg *packages.Package, body *ast.BlockStmt, funcName string, findings *[]Finding) {
	// Collect all open-like calls and all defer close calls in this function scope.
	type openCall struct {
		varName string
		pos     ast.Node
		pattern string
	}

	var opens []openCall
	deferCloseTargets := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.DeferStmt:
			// Track defer X.Close() targets.
			if call, ok := s.Call.Fun.(*ast.SelectorExpr); ok && call.Sel.Name == "Close" {
				if ident, ok := call.X.(*ast.Ident); ok {
					deferCloseTargets[ident.Name] = true
				}
				// Handle defer resp.Body.Close().
				if sel, ok := call.X.(*ast.SelectorExpr); ok {
					if ident, ok := sel.X.(*ast.Ident); ok {
						deferCloseTargets[ident.Name+"."+sel.Sel.Name] = true
					}
				}
			}
		case *ast.AssignStmt:
			// Track resource-opening assignments: f, err := os.Open(...)
			if len(s.Rhs) != 1 {
				return true
			}
			call, ok := s.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check against known openers.
			for _, opener := range knownOpeners {
				if matchesOpener(sel, opener) && len(s.Lhs) >= 1 {
					if ident, ok := s.Lhs[0].(*ast.Ident); ok {
						opens = append(opens, openCall{
							varName: ident.Name,
							pos:     s,
							pattern: fmt.Sprintf("%s.%s", opener.PkgName, sel.Sel.Name),
						})
					}
					break
				}
			}

			// Check HTTP response patterns: resp, err := http.Get(...)
			if isHTTPResponseCall(sel) && len(s.Lhs) >= 1 {
				if ident, ok := s.Lhs[0].(*ast.Ident); ok {
					opens = append(opens, openCall{
						varName: ident.Name,
						pos:     s,
						pattern: fmt.Sprintf("http.%s", sel.Sel.Name),
					})
				}
			}
		}
		return true
	})

	// Report opens that have no matching defer close.
	for _, o := range opens {
		if deferCloseTargets[o.varName] {
			continue
		}
		// For HTTP responses, check resp.Body.
		if strings.HasPrefix(o.pattern, "http.") {
			if deferCloseTargets[o.varName+".Body"] {
				continue
			}
		}
		pos := pkg.Fset.Position(o.pos.Pos())
		severity := "HIGH"
		suggestion := fmt.Sprintf("Add `defer %s.Close()` after error check.", o.varName)
		if strings.HasPrefix(o.pattern, "http.") {
			suggestion = fmt.Sprintf("Add `defer %s.Body.Close()` after error check.", o.varName)
		}

		*findings = append(*findings, Finding{
			File:        pos.Filename,
			Line:        pos.Line,
			Function:    funcName,
			Category:    "resource_leak",
			Severity:    severity,
			Pattern:     "missing_defer_close",
			Description: fmt.Sprintf("Resource opened via %s but no defer Close() found in function scope.", o.pattern),
			Suggestion:  suggestion,
		})
	}
}

// detectDeferInLoop flags defer statements inside loop bodies.
func detectDeferInLoop(pkg *packages.Package, body *ast.BlockStmt, funcName string, inLoop bool, findings *[]Finding) {
	for _, stmt := range body.List {
		switch s := stmt.(type) {
		case *ast.DeferStmt:
			if inLoop {
				pos := pkg.Fset.Position(s.Pos())
				*findings = append(*findings, Finding{
					File:        pos.Filename,
					Line:        pos.Line,
					Function:    funcName,
					Category:    "resource_leak",
					Severity:    "WARNING",
					Pattern:     "defer_in_loop",
					Description: "defer statement inside a loop — deferred calls accumulate and are not released until the function returns.",
					Suggestion:  "Move the resource handling into a separate function, or close the resource explicitly within the loop body.",
				})
			}
		case *ast.ForStmt:
			if s.Body != nil {
				detectDeferInLoop(pkg, s.Body, funcName, true, findings)
			}
		case *ast.RangeStmt:
			if s.Body != nil {
				detectDeferInLoop(pkg, s.Body, funcName, true, findings)
			}
		case *ast.IfStmt:
			if s.Body != nil {
				detectDeferInLoop(pkg, s.Body, funcName, inLoop, findings)
			}
			if s.Else != nil {
				if block, ok := s.Else.(*ast.BlockStmt); ok {
					detectDeferInLoop(pkg, block, funcName, inLoop, findings)
				}
			}
		case *ast.SwitchStmt:
			if s.Body != nil {
				detectDeferInLoop(pkg, s.Body, funcName, inLoop, findings)
			}
		}
	}
}

// matchesOpener checks if a selector expression matches a known resource opener.
func matchesOpener(sel *ast.SelectorExpr, opener resourceOpener) bool {
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != opener.PkgName {
		return false
	}
	if sel.Sel.Name == opener.FuncName {
		return true
	}
	for _, alias := range opener.Aliases {
		if sel.Sel.Name == alias {
			return true
		}
	}
	return false
}

// isHTTPResponseCall checks if a selector is an HTTP method returning a response.
func isHTTPResponseCall(sel *ast.SelectorExpr) bool {
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	if ident.Name != "http" {
		return false
	}
	for _, method := range httpResponseMethods {
		if sel.Sel.Name == method {
			return true
		}
	}
	return false
}
