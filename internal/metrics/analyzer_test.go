package metrics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTool_Metadata(t *testing.T) {
	tool := &Tool{}
	if tool.Name() != "go_complexity_analyzer" {
		t.Errorf("expected name go_complexity_analyzer, got %s", tool.Name())
	}
}

func TestTool_Handle(t *testing.T) {
	tool := &Tool{}
	ctx := context.Background()

	// Case 1: Valid package
	input := ComplexityInput{
		Pkg: ".",
	}
	req := &mcp.CallToolRequest{}

	resp, _, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed unexpectedly: %v", err)
	}
	if resp.IsError {
		t.Errorf("expected success, got error content")
	}

	// Verify header in output
	headerFound := false
	for _, c := range resp.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if strings.Contains(tc.Text, "Complexity analysis for package") {
				headerFound = true
				break
			}
		}
	}
	if !headerFound {
		t.Error("response content missing complexity analysis header")
	}

	// Case 2: Missing package (error handling)
	inputErr := ComplexityInput{
		Pkg: "./non-existent-dir-12345",
	}
	respErr, _, err := tool.Handle(ctx, req, inputErr)
	if err != nil {
		t.Fatalf("Handle failed unexpectedly: %v", err)
	}
	if !respErr.IsError {
		t.Error("expected error for non-existent directory, got success")
	}
}

func TestCalculateComplexity_Table(t *testing.T) {
	// Create a temporary directory with Go files of varying complexity
	tmp, err := os.MkdirTemp("", "metrics-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	// Simple module scaffold
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module testmetrics\n\ngo 1.20\n"), 0644)
	
	code := `
package testmetrics

func Simple() int {
	return 1
}

func ComplexFlow(a, b bool) int {
	if a {
		if b {
			return 2
		}
		return 3
	}
	return 4
}
`
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte(code), 0644)

	result, err := CalculateComplexity(context.Background(), tmp)
	if err != nil {
		t.Fatalf("CalculateComplexity failed: %v", err)
	}

	// Verify we got results
	if len(result.Functions) < 2 {
		t.Errorf("expected at least 2 functions, got %d", len(result.Functions))
	}

	// Check ComplexFlow specifically
	if m, ok := result.Functions["ComplexFlow"]; ok {
		if m.Cyclomatic != 3 {
			t.Errorf("ComplexFlow: expected cyclomatic 3, got %d", m.Cyclomatic)
		}
		if m.Cognitive != 3 {
			t.Errorf("ComplexFlow: expected cognitive 3, got %d", m.Cognitive)
		}
	} else {
		t.Error("ComplexFlow not found in results")
	}
}
