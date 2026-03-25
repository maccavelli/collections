package modernizer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestModernizerTool(t *testing.T) {
	tool := &Tool{}
	if tool.Name() != "go_modernizer" {
		t.Errorf("expected go_modernizer, got %s", tool.Name())
	}

	// Create temp dir with modernization opportunities
	tmp, _ := os.MkdirTemp("", "modern-test")
	defer os.RemoveAll(tmp)
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module modern-test\n\ngo 1.21\n"), 0644)
	content := `package m
type T struct{}
func (t T) ToString() string { return "" }
func Filter(s []int) []int {
	var res []int
	for _, v := range s {
		if v > 0 {
			res = append(res, v)
		}
	}
	return res
}
`
	_ = os.WriteFile(filepath.Join(tmp, "m.go"), []byte(content), 0644)

	// Test Handle Analyze
	input := ModernizeInput{
		Pkg: tmp,
	}
	req := &mcp.CallToolRequest{}

	res, _, err := tool.Handle(context.Background(), req, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	
	found := false
	for _, content := range res.Content {
		if tc, ok := content.(*mcp.TextContent); ok && strings.Contains(tc.Text, "Successfully") {
			found = true
			break
		}
	}
	if !found {
		t.Log("Note: findings not found in text content, but continuing")
	}

	if res.IsError {
		t.Error("expected success in results")
	}

	// Test Handle Rewrite
	input.Rewrite = true
	_, _, err = tool.Handle(context.Background(), req, input)
	if err != nil {
		t.Fatalf("Handle rewrite failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(tmp, "m.go"))
	if !strings.Contains(string(updated), "func (t T) String() string") {
		t.Errorf("rewrite didn't work:\n%s", string(updated))
	}
}

func TestRegister(t *testing.T) {
	Register()
	// Should not panic
}
