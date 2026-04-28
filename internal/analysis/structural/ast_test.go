package structural

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/buntdb"
)

func TestInspector_AnalyzeDirectory_NoFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	i := NewInspector(db)
	gaps, err := i.AnalyzeDirectory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("AnalyzeDirectory failed: %v", err)
	}
	if len(gaps) != 0 {
		t.Errorf("expected 0 gaps, got %d", len(gaps))
	}
}

func TestInspector_CheckFuncSize(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-test-ast-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	var sb strings.Builder
	sb.WriteString("package test\nfunc BigFunc() {\n")
	for j := 0; j < 110; j++ {
		sb.WriteString("\t// dummy\n")
	}
	sb.WriteString("}\n")

	err = os.WriteFile(filepath.Join(tmpDir, "big.go"), []byte(sb.String()), 0644)
	if err != nil {
		t.Fatal(err)
	}

	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	i := NewInspector(db)
	gaps, err := i.AnalyzeDirectory(context.Background(), tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, g := range gaps {
		if g.Area == "CODE_COMPLEXITY" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected CODE_COMPLEXITY gap for large function")
	}
}

func TestInspector_CheckContextParam(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-test-ctx-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	content := `package test
func AnalyzeSomething() { } 
func HandleRequest(x int) { }
`
	err = os.WriteFile(filepath.Join(tmpDir, "ctx.go"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	i := NewInspector(db)
	gaps, err := i.AnalyzeDirectory(context.Background(), tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	analyzeFound := false
	handleFound := false
	for _, g := range gaps {
		if g.Area == "STABILITY" {
			if strings.Contains(g.Description, "AnalyzeSomething") {
				analyzeFound = true
			}
			if strings.Contains(g.Description, "HandleRequest") {
				handleFound = true
			}
		}
	}
	if !analyzeFound {
		t.Error("expected gap for AnalyzeSomething")
	}
	if !handleFound {
		t.Error("expected gap for HandleRequest")
	}
}

func TestInspector_CheckSuppressedError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-test-err-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	content := `package test
import "fmt"
func F() {
	_ = fmt.Println("test")
}
`
	err = os.WriteFile(filepath.Join(tmpDir, "err.go"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	i := NewInspector(db)
	gaps, err := i.AnalyzeDirectory(context.Background(), tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, g := range gaps {
		if g.Area == "STABILITY" && strings.Contains(strings.ToLower(g.Description), "suppressed error") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected suppressed error gap")
	}
}
func TestInspector_AnalyzeDirectory_Cancelled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-test-cancel-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	content := `package test
func F() {}
`
	err = os.WriteFile(filepath.Join(tmpDir, "f.go"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	i := NewInspector(db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = i.AnalyzeDirectory(ctx, tmpDir)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled error, got: %v", err)
	}
}

func TestInspector_CheckBlankErrorAssign_Define(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "mcp-ast-define-*")
	defer os.RemoveAll(tmpDir)

	content := `package test
func F() {
	x, _ := someFunc()
	println(x)
}
`
	_ = os.WriteFile(filepath.Join(tmpDir, "define.go"), []byte(content), 0644)
	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	i := NewInspector(db)
	gaps, _ := i.AnalyzeDirectory(context.Background(), tmpDir)

	found := false
	for _, g := range gaps {
		if g.Area == "STABILITY" && strings.Contains(g.Description, "blank identifier") {
			found = true
		}
	}
	if !found {
		t.Error("expected gap for blank identifier in define statement")
	}
}

func TestInspector_CheckContextParam_WrongType(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "mcp-ast-ctx-err-*")
	defer os.RemoveAll(tmpDir)

	content := `package test
type MyContext struct{}
func AnalyzeMistake(ctx MyContext) {}
`
	_ = os.WriteFile(filepath.Join(tmpDir, "wrong_ctx.go"), []byte(content), 0644)
	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	i := NewInspector(db)
	gaps, _ := i.AnalyzeDirectory(context.Background(), tmpDir)

	found := false
	for _, g := range gaps {
		if g.Area == "STABILITY" && strings.Contains(g.Description, "lacks context.Context") {
			found = true
		}
	}
	if !found {
		t.Error("expected gap for wrong context type")
	}
}

func TestInspector_AnalyzeDirectory_NonExistent(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	i := NewInspector(db)
	_, err := i.AnalyzeDirectory(context.Background(), "/non/existent/path/for/mcp/test")
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestInspector_CheckSecrets(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "mcp-ast-secrets-*")
	defer os.RemoveAll(tmpDir)

	content := `package test
const RiskLevel = "risk_level"
const SecretKey = "sk_live_1234567890abcdef"
const APIKey = "my_api_key_xxxxxxxx"
`
	_ = os.WriteFile(filepath.Join(tmpDir, "secrets.go"), []byte(content), 0644)
	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	i := NewInspector(db)
	gaps, _ := i.AnalyzeDirectory(context.Background(), tmpDir)

	riskFound := false
	secretFound := false
	apiFound := false

	for _, g := range gaps {
		if g.Area == "SECURITY" {
			if strings.Contains(g.Description, "secrets.go:2") {
				riskFound = true
			}
			if strings.Contains(g.Description, "secrets.go:3") {
				secretFound = true
			}
			if strings.Contains(g.Description, "secrets.go:4") {
				apiFound = true
			}
		}
	}

	if riskFound {
		t.Error("false positive: flagged 'risk_level' as secret")
	}
	if !secretFound {
		t.Error("failed to detect 'sk_live_...' as secret")
	}
	if !apiFound {
		t.Error("failed to detect 'my_api_key_...' as secret")
	}
}

func TestInspector_CachingAndMetrics(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "mcp-ast-cache-*")
	defer os.RemoveAll(tmpDir)

	content := "package test\nfunc F() {}\n"
	path := filepath.Join(tmpDir, "f.go")
	_ = os.WriteFile(path, []byte(content), 0644)

	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	i := NewInspector(db)

	ctx := context.Background()

	// 1. Initial analysis (Miss)
	_, _ = i.AnalyzeDirectory(ctx, tmpDir)
	hits, misses, entries := i.GetMetrics()
	if hits != 0 || misses != 1 {
		t.Errorf("expected 0 hits, 1 miss; got %d hits, %d misses", hits, misses)
	}
	if entries != 2 { // One for the file, one for the aggregate cache
		t.Errorf("expected 2 entries in cache, got %d", entries)
	}

	// 2. Second analysis (Aggregate Hit - processFile is bypassed)
	_, _ = i.AnalyzeDirectory(ctx, tmpDir)
	hits, misses, entries = i.GetMetrics()
	if hits != 0 || misses != 1 {
		t.Errorf("expected 0 hits, 1 miss; got %d hits, %d misses", hits, misses)
	}

	// Delete aggregate cache to force file-level fallback
	_ = db.Update(func(tx *buntdb.Tx) error {
		absRoot, _ := filepath.Abs(tmpDir)
		aggKey := fmt.Sprintf("brainstorm:ast_agg:%x", sha256.Sum256([]byte(absRoot)))
		_, _ = tx.Delete(aggKey)
		return nil
	})

	// 3. Third analysis (Aggregate Miss, File Hit)
	_, _ = i.AnalyzeDirectory(ctx, tmpDir)
	hits, misses, entries = i.GetMetrics()
	if hits != 1 || misses != 1 {
		t.Errorf("expected 1 hit, 1 miss; got %d hits, %d misses", hits, misses)
	}

	// 4. Modify file (Aggregate Miss due to delete, File Miss due to hash change)
	_ = db.Update(func(tx *buntdb.Tx) error {
		absRoot, _ := filepath.Abs(tmpDir)
		aggKey := fmt.Sprintf("brainstorm:ast_agg:%x", sha256.Sum256([]byte(absRoot)))
		_, _ = tx.Delete(aggKey)
		return nil
	})
	_ = os.WriteFile(path, []byte(content+"// change\n"), 0644)
	_, _ = i.AnalyzeDirectory(ctx, tmpDir)
	hits, misses, entries = i.GetMetrics()
	if hits != 1 || misses != 2 {
		t.Errorf("expected 1 hit, 2 misses; got %d hits, %d misses", hits, misses)
	}
}
