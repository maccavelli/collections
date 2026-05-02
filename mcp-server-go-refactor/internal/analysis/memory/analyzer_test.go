package memory

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

// buildTestPackage creates a synthetic *packages.Package from raw Go source code
// for test isolation without requiring filesystem access.
func buildTestPackage(t *testing.T, src string) []*packages.Package {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "testfile.go", src, parser.AllErrors)
	if err != nil {
		t.Fatalf("failed to parse test source: %v", err)
	}

	conf := types.Config{
		Importer: nil, // No imports needed for pure AST pattern tests.
		Error:    func(err error) { /* Ignore type-check errors from missing imports. */ },
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Uses:  make(map[*ast.Ident]types.Object),
		Defs:  make(map[*ast.Ident]types.Object),
	}

	_, _ = conf.Check("testpkg", fset, []*ast.File{file}, info)

	return []*packages.Package{{
		Fset:      fset,
		Syntax:    []*ast.File{file},
		TypesInfo: info,
	}}
}

// --------------------------------------------------------------------------
// Goroutine Leak Detection Tests
// --------------------------------------------------------------------------

func TestDetectGoroutineInLoop(t *testing.T) {
	src := `package testpkg

func leaky() {
	for i := 0; i < 10; i++ {
		go func() {
			_ = i
		}()
	}
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectGoroutineLeaks(pkgs)

	found := false
	for _, f := range findings {
		if f.Pattern == "goroutine_in_loop" {
			found = true
			if f.Severity != "CRITICAL" {
				t.Errorf("expected CRITICAL severity, got %s", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected goroutine_in_loop finding, got none")
	}
}

func TestDetectGoroutineNoContext(t *testing.T) {
	src := `package testpkg

func noCtx() {
	go func() {
		println("fire and forget")
	}()
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectGoroutineLeaks(pkgs)

	found := false
	for _, f := range findings {
		if f.Pattern == "goroutine_no_context" {
			found = true
		}
	}
	if !found {
		t.Error("expected goroutine_no_context finding, got none")
	}
}

func TestGoroutineWithContextNotFlagged(t *testing.T) {
	src := `package testpkg

func withCtx() {
	go func() {
		_ = ctx
	}()
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectGoroutineLeaks(pkgs)

	for _, f := range findings {
		if f.Pattern == "goroutine_no_context" {
			t.Error("goroutine capturing ctx should not be flagged as goroutine_no_context")
		}
	}
}

func TestTimeAfterInLoop(t *testing.T) {
	src := `package testpkg

import "time"

func loopTimer() {
	for {
		select {
		case <-time.After(time.Second):
		}
	}
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectGoroutineLeaks(pkgs)

	found := false
	for _, f := range findings {
		if f.Pattern == "time_after_in_loop" {
			found = true
		}
	}
	if !found {
		t.Error("expected time_after_in_loop finding, got none")
	}
}

// --------------------------------------------------------------------------
// Resource Leak Detection Tests
// --------------------------------------------------------------------------

func TestDetectMissingDeferClose(t *testing.T) {
	src := `package testpkg

import "os"

func leakyFile() {
	f, _ := os.Open("test.txt")
	_ = f
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectResourceLeaks(pkgs)

	found := false
	for _, f := range findings {
		if f.Pattern == "missing_defer_close" {
			found = true
		}
	}
	if !found {
		t.Error("expected missing_defer_close finding, got none")
	}
}

func TestDeferCloseNotFlagged(t *testing.T) {
	src := `package testpkg

import "os"

func safeFile() {
	f, _ := os.Open("test.txt")
	defer f.Close()
	_ = f
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectResourceLeaks(pkgs)

	for _, f := range findings {
		if f.Pattern == "missing_defer_close" {
			t.Error("file with defer Close() should not be flagged")
		}
	}
}

func TestDeferInLoop(t *testing.T) {
	src := `package testpkg

import "os"

func loopDefer() {
	for i := 0; i < 10; i++ {
		f, _ := os.Open("test.txt")
		defer f.Close()
		_ = f
	}
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectResourceLeaks(pkgs)

	foundDeferInLoop := false
	for _, f := range findings {
		if f.Pattern == "defer_in_loop" {
			foundDeferInLoop = true
		}
	}
	if !foundDeferInLoop {
		t.Error("expected defer_in_loop finding, got none")
	}
}

// --------------------------------------------------------------------------
// Allocation Issue Detection Tests
// --------------------------------------------------------------------------

func TestAppendInLoopNoPrealloc(t *testing.T) {
	src := `package testpkg

func growSlice() {
	var items []string
	for i := 0; i < 100; i++ {
		items = append(items, "item")
	}
	_ = items
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectAllocationIssues(pkgs)

	found := false
	for _, f := range findings {
		if f.Pattern == "append_in_loop_no_prealloc" {
			found = true
		}
	}
	if !found {
		t.Error("expected append_in_loop_no_prealloc finding, got none")
	}
}

func TestAppendWithPreallocNotFlagged(t *testing.T) {
	src := `package testpkg

func preallocSlice() {
	items := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		items = append(items, "item")
	}
	_ = items
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectAllocationIssues(pkgs)

	for _, f := range findings {
		if f.Pattern == "append_in_loop_no_prealloc" {
			t.Error("pre-allocated slice should not be flagged")
		}
	}
}

func TestSprintfSimpleConcat(t *testing.T) {
	src := `package testpkg

import "fmt"

func concat(a, b string) string {
	return fmt.Sprintf("%s%s", a, b)
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectAllocationIssues(pkgs)

	found := false
	for _, f := range findings {
		if f.Pattern == "sprintf_simple_concat" {
			found = true
		}
	}
	if !found {
		t.Error("expected sprintf_simple_concat finding, got none")
	}
}

// --------------------------------------------------------------------------
// Escape Analysis Tests
// --------------------------------------------------------------------------

func TestPointerReturnEscape(t *testing.T) {
	src := `package testpkg

type Config struct {
	Name string
}

func NewConfig() *Config {
	return &Config{Name: "test"}
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectEscapeIssues(pkgs)

	found := false
	for _, f := range findings {
		if f.Pattern == "pointer_return_escape" {
			found = true
		}
	}
	if !found {
		t.Error("expected pointer_return_escape finding, got none")
	}
}

// --------------------------------------------------------------------------
// Sync.Pool Tests
// --------------------------------------------------------------------------

func TestPoolGetNoReset(t *testing.T) {
	src := `package testpkg

import "sync"

var pool = sync.Pool{New: func() any { return &bytes{} }}

type bytes struct{ data []byte }

func usePool() {
	buf := pool.Get()
	_ = buf
	pool.Put(buf)
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectSyncPoolIssues(pkgs)

	found := false
	for _, f := range findings {
		if f.Pattern == "pool_get_no_reset" {
			found = true
		}
	}
	if !found {
		t.Error("expected pool_get_no_reset finding, got none")
	}
}

// --------------------------------------------------------------------------
// Modernizer Tests
// --------------------------------------------------------------------------

func TestManualGC(t *testing.T) {
	src := `package testpkg

import "runtime"

func forceGC() {
	runtime.GC()
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectModernPatterns(pkgs)

	found := false
	for _, f := range findings {
		if f.Pattern == "manual_gc_call" {
			found = true
		}
	}
	if !found {
		t.Error("expected manual_gc_call finding, got none")
	}
}

func TestManualZeroLoop(t *testing.T) {
	src := `package testpkg

func zeroSlice(s []int) {
	for i := range s {
		s[i] = 0
	}
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectModernPatterns(pkgs)

	found := false
	for _, f := range findings {
		if f.Pattern == "manual_zero_loop" {
			found = true
		}
	}
	if !found {
		t.Error("expected manual_zero_loop finding, got none")
	}
}

func TestFinalizerNoKeepAlive(t *testing.T) {
	src := `package testpkg

import "runtime"

type Resource struct{}

func newResource() *Resource {
	r := &Resource{}
	runtime.SetFinalizer(r, func(r *Resource) {})
	return r
}`
	pkgs := buildTestPackage(t, src)
	findings := DetectModernPatterns(pkgs)

	found := false
	for _, f := range findings {
		if f.Pattern == "finalizer_no_keepalive" {
			found = true
		}
	}
	if !found {
		t.Error("expected finalizer_no_keepalive finding, got none")
	}
}

// --------------------------------------------------------------------------
// Composite Analyzer Integration Test
// --------------------------------------------------------------------------

func TestMemoryAnalysisResult_TotalFindings(t *testing.T) {
	result := &MemoryAnalysisResult{
		GoroutineLeaks: []Finding{
			{Pattern: "goroutine_in_loop"},
			{Pattern: "goroutine_no_context"},
		},
		ResourceLeaks:    []Finding{{Pattern: "missing_defer_close"}},
		AllocationIssues: []Finding{{Pattern: "append_in_loop_no_prealloc"}},
		EscapeHints:      []Finding{{Pattern: "pointer_return_escape"}},
		SyncPoolIssues:   nil,
		ModernPatterns:   []Finding{{Pattern: "manual_gc_call"}},
	}

	if got := result.TotalFindings(); got != 6 {
		t.Errorf("TotalFindings() = %d, want 6", got)
	}
}

func TestFormatSummary(t *testing.T) {
	result := &MemoryAnalysisResult{
		GoroutineLeaks: []Finding{{Pattern: "test"}},
		ResourceLeaks:  []Finding{{Pattern: "test"}, {Pattern: "test2"}},
	}

	summary := formatSummary("example/pkg", result)
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}
