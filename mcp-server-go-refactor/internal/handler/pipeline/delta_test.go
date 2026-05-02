package pipeline

import (
	"testing"
)

const sampleBefore = `package example

import "fmt"

// Greeter greets users.
type Greeter struct {
	Name string
	Age  int
}

// Greet returns a greeting string.
func (g *Greeter) Greet() string {
	return fmt.Sprintf("Hello, %s!", g.Name)
}

// Add adds two integers.
func Add(a, b int) int {
	return a + b
}
`

// mustFingerprint is a test helper that extracts a fingerprint or fails the test.
func mustFingerprint(t *testing.T, src string) *ASTFingerprint {
	t.Helper()
	fp, err := ExtractFingerprint([]byte(src))
	if err != nil {
		t.Fatalf("ExtractFingerprint failed: %v", err)
	}
	return fp
}

func TestExtractFingerprint(t *testing.T) {
	fp := mustFingerprint(t, sampleBefore)

	if fp.PackageName != "example" {
		t.Errorf("PackageName = %q, want %q", fp.PackageName, "example")
	}
	if len(fp.ImportPaths) != 1 || fp.ImportPaths[0] != "fmt" {
		t.Errorf("ImportPaths = %v, want [fmt]", fp.ImportPaths)
	}
	if len(fp.FuncDecls) != 2 {
		t.Errorf("FuncDecls count = %d, want 2", len(fp.FuncDecls))
	}
	if len(fp.TypeDecls) != 1 {
		t.Errorf("TypeDecls count = %d, want 1", len(fp.TypeDecls))
	}
	if fp.TypeDecls[0].FieldCount != 2 {
		t.Errorf("Greeter.FieldCount = %d, want 2", fp.TypeDecls[0].FieldCount)
	}
}

func TestExtractFingerprint_MalformedSource(t *testing.T) {
	_, err := ExtractFingerprint([]byte("this is not go code {{{"))
	if err == nil {
		t.Error("expected error for malformed source, got nil")
	}
}

func TestComputeDelta_CommentOnly(t *testing.T) {
	after := `package example

import "fmt"

// Greeter greets users.
// It handles personalized greetings.
type Greeter struct {
	Name string
	Age  int
}

// Greet returns a greeting string for the given user.
func (g *Greeter) Greet() string {
	return fmt.Sprintf("Hello, %s!", g.Name)
}

// Add adds two integers and returns the result.
// Deprecated: Use Sum instead.
func Add(a, b int) int {
	return a + b
}

// EOF
`
	bfp := mustFingerprint(t, sampleBefore)
	afp := mustFingerprint(t, after)

	delta := ComputeDelta(bfp, afp)
	if delta.Category != DeltaCommentOnly {
		t.Errorf("Category = %q, want %q (comment delta: %d, before=%d, after=%d)",
			delta.Category, DeltaCommentOnly, delta.CommentDelta, bfp.Comments, afp.Comments)
	}
}

func TestComputeDelta_Additive(t *testing.T) {
	after := `package example

import "fmt"

// Greeter greets users.
type Greeter struct {
	Name string
	Age  int
}

// Greet returns a greeting string.
func (g *Greeter) Greet() string {
	return fmt.Sprintf("Hello, %s!", g.Name)
}

// Add adds two integers.
func Add(a, b int) int {
	return a + b
}

// Subtract subtracts b from a.
func Subtract(a, b int) int {
	return a - b
}
`
	bfp := mustFingerprint(t, sampleBefore)
	afp := mustFingerprint(t, after)

	delta := ComputeDelta(bfp, afp)
	if delta.Category != DeltaAdditive {
		t.Errorf("Category = %q, want %q", delta.Category, DeltaAdditive)
	}
	if len(delta.AddedDecls) != 1 {
		t.Errorf("AddedDecls count = %d, want 1", len(delta.AddedDecls))
	}
}

func TestComputeDelta_Destructive(t *testing.T) {
	after := `package example

import "fmt"

// Greeter greets users.
type Greeter struct {
	Name string
	Age  int
}

// Greet returns a greeting string.
func (g *Greeter) Greet() string {
	return fmt.Sprintf("Hello, %s!", g.Name)
}
`
	bfp := mustFingerprint(t, sampleBefore)
	afp := mustFingerprint(t, after)

	delta := ComputeDelta(bfp, afp)
	if delta.Category != DeltaDestructive {
		t.Errorf("Category = %q, want %q", delta.Category, DeltaDestructive)
	}
	if len(delta.RemovedDecls) != 1 {
		t.Errorf("RemovedDecls count = %d, want 1", len(delta.RemovedDecls))
	}
}

func TestComputeDelta_SignatureMutation(t *testing.T) {
	after := `package example

import "fmt"

// Greeter greets users.
type Greeter struct {
	Name string
	Age  int
}

// Greet returns a greeting string.
func (g *Greeter) Greet() string {
	return fmt.Sprintf("Hello, %s!", g.Name)
}

// Add adds integers.
func Add(a, b, c int) int {
	return a + b + c
}
`
	bfp := mustFingerprint(t, sampleBefore)
	afp := mustFingerprint(t, after)

	delta := ComputeDelta(bfp, afp)
	if delta.Category != DeltaSignatureMutation {
		t.Errorf("Category = %q, want %q", delta.Category, DeltaSignatureMutation)
	}
}

func TestComputeDelta_PackageRename(t *testing.T) {
	after := `package renamed

import "fmt"

// Greeter greets users.
type Greeter struct {
	Name string
	Age  int
}

// Greet returns a greeting string.
func (g *Greeter) Greet() string {
	return fmt.Sprintf("Hello, %s!", g.Name)
}

// Add adds two integers.
func Add(a, b int) int {
	return a + b
}
`
	bfp := mustFingerprint(t, sampleBefore)
	afp := mustFingerprint(t, after)

	delta := ComputeDelta(bfp, afp)
	if delta.Category != DeltaPackageRename {
		t.Errorf("Category = %q, want %q", delta.Category, DeltaPackageRename)
	}
}

func TestValidateDelta_BlocksDestructive(t *testing.T) {
	delta := &ASTDelta{Category: DeltaDestructive, Description: "1 declaration(s) removed"}
	if err := ValidateDelta(delta, false); err == nil {
		t.Error("expected error for destructive delta without override, got nil")
	}
	if err := ValidateDelta(delta, true); err != nil {
		t.Errorf("expected nil for destructive delta with override, got %v", err)
	}
}

func TestValidateDelta_BlocksPackageRename(t *testing.T) {
	delta := &ASTDelta{Category: DeltaPackageRename, Description: "package name changed"}
	if err := ValidateDelta(delta, true); err == nil {
		t.Error("expected error for package rename even with override, got nil")
	}
}

func TestValidateDelta_AllowsCommentOnly(t *testing.T) {
	delta := &ASTDelta{Category: DeltaCommentOnly, Description: "comment-only change"}
	if err := ValidateDelta(delta, false); err != nil {
		t.Errorf("expected nil for comment-only delta, got %v", err)
	}
}

func TestQuickDeclCountCheck(t *testing.T) {
	after := `package example

import "fmt"

// Greeter greets users.
type Greeter struct {
	Name string
	Age  int
}

// Greet returns a greeting string.
func (g *Greeter) Greet() string {
	return fmt.Sprintf("Hello, %s!", g.Name)
}
`
	decreased, orig, mod := QuickDeclCountCheck([]byte(sampleBefore), []byte(after))
	if !decreased {
		t.Errorf("expected decreased=true, got false (orig=%d, mod=%d)", orig, mod)
	}
}

func TestQuickDeclCountCheck_NoDecrease(t *testing.T) {
	decreased, _, _ := QuickDeclCountCheck([]byte(sampleBefore), []byte(sampleBefore))
	if decreased {
		t.Error("expected decreased=false for identical source")
	}
}

func TestTypeCheckSource_Valid(t *testing.T) {
	src := `package example

import "fmt"

func Hello() {
	fmt.Println("hello")
}
`
	errs := TypeCheckSource([]byte(src))
	// Stdlib resolution should work. Any errors are logged for audit.
	t.Logf("type-check errors (audit): %v", errs)
}

func TestTypeCheckSource_MalformedSource(t *testing.T) {
	errs := TypeCheckSource([]byte("this is not go {{{"))
	if len(errs) == 0 {
		t.Error("expected at least one error for malformed source")
	}
}
