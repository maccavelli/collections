package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspector_AnalyzeDirectory_NoFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	i := NewInspector()
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

	i := NewInspector()
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

	i := NewInspector()
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

	i := NewInspector()
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

	i := NewInspector()
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
	i := NewInspector()
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
	i := NewInspector()
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
	i := NewInspector()
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
	i := NewInspector()
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
