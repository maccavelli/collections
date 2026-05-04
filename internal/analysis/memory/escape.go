// Package memory provides functionality for the memory subsystem.
package memory

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// DetectEscapeIssues analyzes packages for patterns known to cause unnecessary heap escapes.
func DetectEscapeIssues(pkgs []*packages.Package) []Finding {
	var findings []Finding

	for _, pkg := range pkgs {
		sizes := types.SizesFor("gc", "amd64")
		if sizes == nil {
			continue
		}

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
				detectPointerReturnEscape(pkg, fn, funcName, &findings)
				detectLargeValueParams(pkg, fn, funcName, sizes, &findings)
				detectInterfaceBoxing(pkg, fn, funcName, &findings)
				return true
			})
		}
	}
	return findings
}

// detectPointerReturnEscape identifies return statements that take the address
// of a local variable, forcing it to escape to the heap.
func detectPointerReturnEscape(pkg *packages.Package, fn *ast.FuncDecl, funcName string, findings *[]Finding) {
	if fn.Type.Results == nil {
		return
	}

	// Check if function returns a pointer type.
	returnsPointer := false
	for _, field := range fn.Type.Results.List {
		if _, ok := field.Type.(*ast.StarExpr); ok {
			returnsPointer = true
			break
		}
	}
	if !returnsPointer {
		return
	}

	// Check return statements for &localVar patterns.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range ret.Results {
			unary, ok := result.(*ast.UnaryExpr)
			if !ok {
				continue
			}
			if unary.Op.String() != "&" {
				continue
			}
			// Check if the operand is a local composite literal.
			if _, ok := unary.X.(*ast.CompositeLit); ok {
				pos := pkg.Fset.Position(unary.Pos())
				*findings = append(*findings, Finding{
					File:        pos.Filename,
					Line:        pos.Line,
					Function:    funcName,
					Category:    "escape_analysis",
					Severity:    "INFO",
					Pattern:     "pointer_return_escape",
					Description: "Returning pointer to local composite literal — forces heap allocation.",
					Suggestion:  "This is standard Go for constructors. Only optimize if this is on a hot path with high allocation pressure.",
				})
			}
		}
		return true
	})
}

// detectLargeValueParams finds function parameters where large structs are
// passed by value instead of by pointer.
func detectLargeValueParams(pkg *packages.Package, fn *ast.FuncDecl, funcName string, sizes types.Sizes, findings *[]Finding) {
	if fn.Type.Params == nil || pkg.TypesInfo == nil {
		return
	}

	const largeStructThreshold = 64 // bytes

	for _, field := range fn.Type.Params.List {
		// Skip pointer types — already efficient.
		if _, ok := field.Type.(*ast.StarExpr); ok {
			continue
		}

		typ := pkg.TypesInfo.TypeOf(field.Type)
		if typ == nil {
			continue
		}

		// Only check struct types.
		underlying := typ.Underlying()
		if _, ok := underlying.(*types.Struct); !ok {
			continue
		}

		size := sizes.Sizeof(underlying)
		if size >= largeStructThreshold {
			for _, name := range field.Names {
				pos := pkg.Fset.Position(name.Pos())
				*findings = append(*findings, Finding{
					File:        pos.Filename,
					Line:        pos.Line,
					Function:    funcName,
					Category:    "escape_analysis",
					Severity:    "INFO",
					Pattern:     "large_value_param",
					Description: fmt.Sprintf("Parameter '%s' passes struct by value (%d bytes) — consider pointer for large structs.", name.Name, size),
					Suggestion:  fmt.Sprintf("Change parameter to `*%s` to avoid copying %d bytes on each call.", typ.String(), size),
				})
			}
		}
	}
}

// detectInterfaceBoxing finds function calls where concrete values are passed
// to interface{}/any parameters, causing implicit boxing (heap allocation).
func detectInterfaceBoxing(pkg *packages.Package, fn *ast.FuncDecl, funcName string, findings *[]Finding) {
	if pkg.TypesInfo == nil {
		return
	}

	// Track function calls where args are boxed into any/interface{}.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Resolve the called function's type signature.
		var sig *types.Signature
		switch f := call.Fun.(type) {
		case *ast.Ident:
			if obj := pkg.TypesInfo.Uses[f]; obj != nil {
				if fn, ok := obj.Type().(*types.Signature); ok {
					sig = fn
				}
			}
		case *ast.SelectorExpr:
			if obj := pkg.TypesInfo.Uses[f.Sel]; obj != nil {
				if fn, ok := obj.Type().(*types.Signature); ok {
					sig = fn
				}
			}
		}

		if sig == nil || sig.Params() == nil {
			return true
		}

		// Check each arg: if the param is interface{} but the arg is a concrete type.
		for i, arg := range call.Args {
			if i >= sig.Params().Len() {
				break
			}
			paramType := sig.Params().At(i).Type()

			// Check if param is an empty interface (any).
			iface, ok := paramType.Underlying().(*types.Interface)
			if !ok || !iface.Empty() {
				continue
			}

			// Get the arg's concrete type.
			argType := pkg.TypesInfo.TypeOf(arg)
			if argType == nil {
				continue
			}

			// Skip if the arg is already an interface type.
			if _, ok := argType.Underlying().(*types.Interface); ok {
				continue
			}

			// Skip basic types that are small and commonly boxed.
			if basic, ok := argType.(*types.Basic); ok {
				switch basic.Kind() {
				case types.Bool, types.Int, types.String:
					continue
				}
			}

			pos := pkg.Fset.Position(arg.Pos())
			*findings = append(*findings, Finding{
				File:        pos.Filename,
				Line:        pos.Line,
				Function:    funcName,
				Category:    "escape_analysis",
				Severity:    "MEDIUM",
				Pattern:     "interface_boxing",
				Description: fmt.Sprintf("Concrete type '%s' implicitly boxed to any/interface{} — forces heap allocation.", argType.String()),
				Suggestion:  "Consider using a generic function or concrete type parameter to avoid boxing.",
			})
		}
		return true
	})
}
