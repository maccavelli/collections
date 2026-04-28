package tags

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcp-server-go-refactor/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func ptr(s string) *string { return &s }

type TagStruct struct {
	FirstName string
	LastName  string `json:"old_last"`
}

func TestAnalyzeTags(t *testing.T) {
	result, err := AnalyzeTags(context.Background(), ".", "TagStruct", "snake", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected tag modification data")
	}

	found := false
	for _, m := range result.Modifications {
		if m.Field == "FirstName" {
			found = true
			if m.SuggestedTag != "`json:\"first_name\"`" {
				t.Errorf("unexpected suggestion for FirstName: %s", m.SuggestedTag)
			}
		}
	}
	if !found {
		t.Error("FirstName not found in modifications")
	}
}

func TestFormatCase(t *testing.T) {
	tests := []struct {
		input  string
		format string
		want   string
	}{
		{"FirstName", "snake", "first_name"},
		{"FirstName", "camel", "firstName"},
		{"FirstName", "pascal", "FirstName"},
		{"FirstName", "kebab", "first-name"},
		{"HTTPClient", "snake", "h_t_t_p_client"},
		{"", "camel", ""},
		{"Single", "snake", "single"},
		{"Unknown", "none", "unknown"},
	}

	for _, tc := range tests {
		got := formatCase(tc.input, tc.format)
		if got != tc.want {
			t.Errorf("formatCase(%q, %q) = %q; want %q", tc.input, tc.format, got, tc.want)
		}
	}
}

func TestApplyTags(t *testing.T) {
	tmp, err := os.MkdirTemp("", "tags-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module tags-test\n\ngo 1.21\n"), 0644)
	content := "package test\ntype Example struct {\n\tFieldA string\n\tFieldB int `json:\"old_b\"`\n}\n"
	_ = os.WriteFile(filepath.Join(tmp, "example.go"), []byte(content), 0644)

	err = ApplyTags(context.Background(), tmp, "Example", "snake", "json")
	if err != nil {
		t.Fatalf("ApplyTags failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(tmp, "example.go"))
	if !strings.Contains(string(updated), "`json:\"field_a\"`") {
		t.Errorf("FieldA tag not found in:\n%s", string(updated))
	}
}

func TestTool(t *testing.T) {
	tool := &Tool{}
	if tool.Name() != "go_tag_manager" {
		t.Errorf("expected go_tag_manager, got %s", tool.Name())
	}

	// Test Handle
	req := &mcp.CallToolRequest{}
	input := TagInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target:  ".",
			Context: "TagStruct",
			Flags:   map[string]any{"caseFormat": "snake", "targetTag": "json"},
		},
	}
	res, _, err := tool.Handle(context.Background(), req, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if res.IsError {
		t.Fatal("expected no error in Handle result")
	}

	// Test rewrite=true
	tmp, _ := os.MkdirTemp("", "tags-handle")
	defer os.RemoveAll(tmp)
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module tags-handle\n\ngo 1.21\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmp, "example.go"), []byte("package h\ntype MyS struct { F string }\n"), 0644)

	input = TagInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target:  tmp,
			Context: "MyS",
			Flags:   map[string]any{"rewrite": true},
		},
	}
	_, _, err = tool.Handle(context.Background(), req, input)
	if err != nil {
		t.Fatalf("Handle rewrite failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(tmp, "example.go"))
	if !strings.Contains(string(updated), "`json:\"f\"`") {
		t.Errorf("rewrite didn't work:\n%s", string(updated))
	}
}
