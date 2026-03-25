package modernizer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestModernizerTool(t *testing.T) {
	tool := &Tool{}
	meta := tool.Metadata()
	if meta.Name != "go_modernizer" {
		t.Errorf("expected go_modernizer, got %s", meta.Name)
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
	args := map[string]interface{}{
		"pkg": tmp,
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args

	res, err := tool.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	
	if res == nil {
		t.Fatal("result was nil")
	}

	found := false
	for _, content := range res.Content {
		if tc, ok := content.(mcp.TextContent); ok && strings.Contains(tc.Text, "Modernization findings") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected findings in results")
	}

	// Test Handle Rewrite
	args["rewrite"] = true
	req.Params.Arguments = args
	_, err = tool.Handle(context.Background(), req)
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
