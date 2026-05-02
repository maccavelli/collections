// Package pipeline implements the pipeline tools for the go-refactor MCP server.
// This file provides the AST-Delta Semantic Guard, which compares the structural
// fingerprint of the original and proposed source code to detect destructive
// mutations before they reach disk.
package pipeline

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"log/slog"
	"sort"
	"strings"
)

// DeltaCategory classifies the type of structural change between two AST fingerprints.
type DeltaCategory string

const (
	// DeltaCommentOnly indicates only comments changed with no structural differences.
	DeltaCommentOnly DeltaCategory = "COMMENT_ONLY"
	// DeltaAdditive indicates new declarations were added without removing any.
	DeltaAdditive DeltaCategory = "ADDITIVE"
	// DeltaImportChange indicates the import set changed.
	DeltaImportChange DeltaCategory = "IMPORT_CHANGE"
	// DeltaDestructive indicates declarations were removed or signatures changed.
	DeltaDestructive DeltaCategory = "DESTRUCTIVE"
	// DeltaSignatureMutation indicates parameter or return types changed on existing funcs.
	DeltaSignatureMutation DeltaCategory = "SIGNATURE_MUTATION"
	// DeltaPackageRename indicates the package name was changed.
	DeltaPackageRename DeltaCategory = "PACKAGE_RENAME"
	// DeltaIdentical indicates no differences at all.
	DeltaIdentical DeltaCategory = "IDENTICAL"
)

// DeclFingerprint captures the structural signature of a single top-level declaration.
type DeclFingerprint struct {
	Name        string `json:"name"`
	Exported    bool   `json:"exported"`
	Kind        string `json:"kind"` // "func", "struct", "interface", "type_alias", "var", "const"
	ParamCount  int    `json:"param_count,omitempty"`
	ReturnCount int    `json:"return_count,omitempty"`
	FieldCount  int    `json:"field_count,omitempty"`
	HasReceiver bool   `json:"has_receiver,omitempty"`
}

// ASTFingerprint captures the structural identity of a Go source file.
type ASTFingerprint struct {
	PackageName string            `json:"package_name"`
	ImportPaths []string          `json:"import_paths"`
	FuncDecls   []DeclFingerprint `json:"func_decls"`
	TypeDecls   []DeclFingerprint `json:"type_decls"`
	VarDecls    int               `json:"var_decls"`
	ConstDecls  int               `json:"const_decls"`
	Comments    int               `json:"comments"`
}

// ASTDelta describes the structural differences between two fingerprints.
type ASTDelta struct {
	Category       DeltaCategory     `json:"category"`
	RemovedDecls   []DeclFingerprint `json:"removed_decls,omitempty"`
	AddedDecls     []DeclFingerprint `json:"added_decls,omitempty"`
	ChangedDecls   []string          `json:"changed_decls,omitempty"`
	RemovedImports []string          `json:"removed_imports,omitempty"`
	AddedImports   []string          `json:"added_imports,omitempty"`
	CommentDelta   int               `json:"comment_delta"`
	Description    string            `json:"description"`
}

// ExtractFingerprint parses Go source code and extracts its structural fingerprint.
// Returns nil fingerprint and an error if the source cannot be parsed.
func ExtractFingerprint(src []byte) (*ASTFingerprint, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source for fingerprinting: %w", err)
	}

	fp := &ASTFingerprint{
		PackageName: file.Name.Name,
		Comments:    len(file.Comments),
	}

	// Extract imports.
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		fp.ImportPaths = append(fp.ImportPaths, path)
	}
	sort.Strings(fp.ImportPaths)

	// Walk top-level declarations.
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			df := DeclFingerprint{
				Name:     d.Name.Name,
				Exported: d.Name.IsExported(),
				Kind:     "func",
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				df.HasReceiver = true
			}
			if d.Type.Params != nil {
				df.ParamCount = countFields(d.Type.Params)
			}
			if d.Type.Results != nil {
				df.ReturnCount = countFields(d.Type.Results)
			}
			fp.FuncDecls = append(fp.FuncDecls, df)

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					df := DeclFingerprint{
						Name:     s.Name.Name,
						Exported: s.Name.IsExported(),
					}
					switch st := s.Type.(type) {
					case *ast.StructType:
						df.Kind = "struct"
						if st.Fields != nil {
							df.FieldCount = countFields(st.Fields)
						}
					case *ast.InterfaceType:
						df.Kind = "interface"
						if st.Methods != nil {
							df.FieldCount = countFields(st.Methods)
						}
					default:
						df.Kind = "type_alias"
					}
					fp.TypeDecls = append(fp.TypeDecls, df)

				case *ast.ValueSpec:
					switch d.Tok {
					case token.VAR:
						fp.VarDecls += len(s.Names)
					case token.CONST:
						fp.ConstDecls += len(s.Names)
					}
				}
			}
		}
	}

	return fp, nil
}

// ComputeDelta compares two AST fingerprints and returns the classified delta.
func ComputeDelta(before, after *ASTFingerprint) *ASTDelta {
	delta := &ASTDelta{
		CommentDelta: after.Comments - before.Comments,
	}

	// Check package rename first — always blocked.
	if before.PackageName != after.PackageName {
		delta.Category = DeltaPackageRename
		delta.Description = fmt.Sprintf("package name changed from %q to %q", before.PackageName, after.PackageName)
		return delta
	}

	// Check import changes.
	delta.RemovedImports = setDiff(before.ImportPaths, after.ImportPaths)
	delta.AddedImports = setDiff(after.ImportPaths, before.ImportPaths)

	// Check declaration changes.
	beforeFuncs := declMap(before.FuncDecls)
	afterFuncs := declMap(after.FuncDecls)
	beforeTypes := declMap(before.TypeDecls)
	afterTypes := declMap(after.TypeDecls)

	// Find removed and changed funcs.
	for name, bf := range beforeFuncs {
		af, exists := afterFuncs[name]
		if !exists {
			delta.RemovedDecls = append(delta.RemovedDecls, bf)
		} else if bf.ParamCount != af.ParamCount || bf.ReturnCount != af.ReturnCount {
			delta.ChangedDecls = append(delta.ChangedDecls, fmt.Sprintf(
				"func %s: params %d→%d, returns %d→%d",
				name, bf.ParamCount, af.ParamCount, bf.ReturnCount, af.ReturnCount))
		}
	}

	// Find added funcs.
	for name, af := range afterFuncs {
		if _, exists := beforeFuncs[name]; !exists {
			delta.AddedDecls = append(delta.AddedDecls, af)
		}
	}

	// Find removed and changed types.
	for name, bt := range beforeTypes {
		at, exists := afterTypes[name]
		if !exists {
			delta.RemovedDecls = append(delta.RemovedDecls, bt)
		} else if bt.Kind != at.Kind || bt.FieldCount != at.FieldCount {
			delta.ChangedDecls = append(delta.ChangedDecls, fmt.Sprintf(
				"type %s: kind %s→%s, fields %d→%d",
				name, bt.Kind, at.Kind, bt.FieldCount, at.FieldCount))
		}
	}

	// Find added types.
	for name, at := range afterTypes {
		if _, exists := beforeTypes[name]; !exists {
			delta.AddedDecls = append(delta.AddedDecls, at)
		}
	}

	// Classify the delta.
	hasRemovals := len(delta.RemovedDecls) > 0
	hasSignatureChanges := len(delta.ChangedDecls) > 0
	hasAdditions := len(delta.AddedDecls) > 0
	hasImportChanges := len(delta.RemovedImports) > 0 || len(delta.AddedImports) > 0
	hasStructuralChanges := hasRemovals || hasSignatureChanges || hasAdditions || hasImportChanges ||
		before.VarDecls != after.VarDecls || before.ConstDecls != after.ConstDecls

	switch {
	case hasRemovals:
		delta.Category = DeltaDestructive
		delta.Description = fmt.Sprintf("%d declaration(s) removed", len(delta.RemovedDecls))
	case hasSignatureChanges:
		delta.Category = DeltaSignatureMutation
		delta.Description = fmt.Sprintf("%d signature(s) changed", len(delta.ChangedDecls))
	case !hasStructuralChanges && delta.CommentDelta != 0:
		delta.Category = DeltaCommentOnly
		delta.Description = fmt.Sprintf("comment-only change (delta: %+d)", delta.CommentDelta)
	case !hasStructuralChanges && delta.CommentDelta == 0:
		delta.Category = DeltaIdentical
		delta.Description = "no structural changes detected"
	case hasImportChanges && !hasAdditions:
		delta.Category = DeltaImportChange
		delta.Description = fmt.Sprintf("import change: +%d -%d", len(delta.AddedImports), len(delta.RemovedImports))
	default:
		delta.Category = DeltaAdditive
		delta.Description = fmt.Sprintf("%d declaration(s) added", len(delta.AddedDecls))
	}

	return delta
}

// ValidateDelta checks if the computed delta is acceptable given the pipeline intent.
// Returns nil if the delta is acceptable, or an error describing why it was rejected.
func ValidateDelta(delta *ASTDelta, allowDestructive bool) error {
	switch delta.Category {
	case DeltaPackageRename:
		return fmt.Errorf("REJECTED: %s — package rename is always blocked", delta.Description)
	case DeltaDestructive:
		if !allowDestructive {
			return fmt.Errorf("REJECTED: %s — destructive mutations blocked without explicit override", delta.Description)
		}
	case DeltaSignatureMutation:
		if !allowDestructive {
			return fmt.Errorf("REJECTED: %s — signature mutations blocked without explicit override", delta.Description)
		}
	case DeltaCommentOnly, DeltaAdditive, DeltaImportChange, DeltaIdentical:
		// These are always acceptable.
		return nil
	}
	return nil
}

// TypeCheckSource performs a lightweight single-file type-check using go/types.
// It catches undefined identifiers, duplicate declarations, and stdlib import failures.
// Cross-package type errors are expected and logged as warnings, not hard failures.
// Returns collected errors for audit logging.
func TypeCheckSource(src []byte) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "staged.go", src, parser.ParseComments)
	if err != nil {
		return []string{fmt.Sprintf("parse error: %v", err)}
	}

	var collected []string
	cfg := &types.Config{
		Importer: importer.ForCompiler(fset, "source", nil),
		Error: func(err error) {
			collected = append(collected, err.Error())
		},
	}

	// Single-file type check — cross-package imports will produce expected errors
	// that are logged but not treated as hard failures.
	_, _ = cfg.Check("staged", fset, []*ast.File{file}, nil) //nolint:errcheck // errors collected via callback
	return collected
}

// QuickDeclCountCheck is a fast-path pre-check for the orchestrator that only
// compares top-level declaration counts between original and modified source.
// Returns true if the modified source has fewer declarations than the original.
func QuickDeclCountCheck(originalSrc, modifiedSrc []byte) (decreased bool, origCount, modCount int) {
	origFP, err := ExtractFingerprint(originalSrc)
	if err != nil {
		return false, 0, 0 // Cannot parse original — skip check.
	}
	modFP, err := ExtractFingerprint(modifiedSrc)
	if err != nil {
		slog.Warn("[delta] modified source failed to parse — blocking mutation")
		return true, 0, 0 // Modified source doesn't parse — always block.
	}

	origCount = len(origFP.FuncDecls) + len(origFP.TypeDecls) + origFP.VarDecls + origFP.ConstDecls
	modCount = len(modFP.FuncDecls) + len(modFP.TypeDecls) + modFP.VarDecls + modFP.ConstDecls
	return modCount < origCount, origCount, modCount
}

// countFields returns the total number of names across all fields in a field list.
func countFields(fl *ast.FieldList) int {
	if fl == nil {
		return 0
	}
	count := 0
	for _, f := range fl.List {
		if len(f.Names) == 0 {
			count++ // Unnamed parameter (e.g., embedded field or unnamed return).
		} else {
			count += len(f.Names)
		}
	}
	return count
}

// setDiff returns elements in a that are not in b.
func setDiff(a, b []string) []string {
	bSet := make(map[string]struct{}, len(b))
	for _, s := range b {
		bSet[s] = struct{}{}
	}
	var diff []string
	for _, s := range a {
		if _, exists := bSet[s]; !exists {
			diff = append(diff, s)
		}
	}
	return diff
}

// declMap indexes a slice of DeclFingerprints by name for O(1) lookup.
func declMap(decls []DeclFingerprint) map[string]DeclFingerprint {
	m := make(map[string]DeclFingerprint, len(decls))
	for _, d := range decls {
		key := d.Name
		if d.HasReceiver {
			key = "(*T)." + d.Name // Disambiguate methods from package-level funcs.
		}
		m[key] = d
	}
	return m
}
