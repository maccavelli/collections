package system

import (
	"mcp-server-magicskills/internal/state"

	"context"
	"os"
	"path/filepath"
	"testing"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/handler"
	"mcp-server-magicskills/internal/models"
	"mcp-server-magicskills/internal/scanner"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAddRootTool_Handle(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	scn, _ := scanner.NewScanner([]string{"."})
	tool := &AddRootTool{Engine: eng, Scanner: scn}

	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "magicskills-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	skillDir := filepath.Join(tmpDir, ".agent", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0750); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("---\nname: test-skill\n---\n"), 0600); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	input := AddRootInput{Path: tmpDir}

	ctx := context.Background()
	// Test Handle
	res, out, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if res.IsError {
		t.Errorf("unexpected error in result")
	}
	if out == nil {
		t.Fatal("expected non-nil response")
	}

	output := out.(struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	})
	if output.Summary == "" {
		t.Error("expected non-empty summary")
	}

	// Verify Name
	if tool.Name() != "magicskills_add_root" {
		t.Errorf("expected magicskills_add_root, got %s", tool.Name())
	}
}

func TestValidateDepsTool_Handle(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()

	eng.Skills["test"] = &models.Skill{
		Metadata: models.SkillMetadata{
			Name:         "test",
			Requirements: []string{"go", "nonexistent-binary-123"},
		},
	}

	tool := &ValidateDepsTool{Engine: eng}
	ctx := context.Background()

	// Missing name
	res, _, _ := tool.Handle(ctx, &mcp.CallToolRequest{}, ValidateDepsInput{})
	if !res.IsError {
		t.Error("expected error for missing name")
	}

	// Skill not found
	res, _, _ = tool.Handle(ctx, &mcp.CallToolRequest{}, ValidateDepsInput{Name: "missing"})
	if !res.IsError {
		t.Error("expected error for missing skill")
	}

	// Valid skill with mixed requirements
	res, out, err := tool.Handle(ctx, &mcp.CallToolRequest{}, ValidateDepsInput{Name: "test"})
	if err != nil || res.IsError {
		t.Fatalf("Handle failed: %v", err)
	}
	output := out.(struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	})
	if !strings.Contains(output.Summary, "Missing dependencies") {
		t.Errorf("Expected missing deps in summary, got: %s", output.Summary)
	}
}

func TestGetInternalLogsTool_Handle(t *testing.T) {
	lb := &handler.LogBuffer{}
	_, _ = lb.Write([]byte("line1\nline2\nline3\n"))

	tool := &GetInternalLogsTool{Buffer: lb}

	// Default lines
	res, _, _ := tool.Handle(context.Background(), &mcp.CallToolRequest{}, LogsInput{})
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "line3") {
		t.Error("Expected log lines in output")
	}

	// Max lines
	res, _, _ = tool.Handle(context.Background(), &mcp.CallToolRequest{}, LogsInput{MaxLines: 1})
	text = res.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "line1") {
		t.Error("Expected only 1 line, got more")
	}
}

func TestHealthTool_Handle(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()

	tool := &HealthTool{Engine: eng}
	res, out, _ := tool.Handle(context.Background(), &mcp.CallToolRequest{}, HealthInput{})
	if res.IsError {
		t.Error("HealthTool failed")
	}
	output := out.(struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	})
	if !strings.Contains(output.Summary, "healthy") {
		t.Errorf("expected healthy summary, got %s", output.Summary)
	}
}

func TestAddRootTool_Errors(t *testing.T) {
	tool := &AddRootTool{}
	res, _, _ := tool.Handle(context.Background(), &mcp.CallToolRequest{}, AddRootInput{Path: "/nonexistent"})
	if !res.IsError {
		t.Error("expected error for invalid path")
	}
}
func TestRegister(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	scn, _ := scanner.NewScanner([]string{"."})
	lb := &handler.LogBuffer{}

	Register(eng, scn, lb)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, &mcp.ServerOptions{})

	addRoot := &AddRootTool{Scanner: scn, Engine: eng}
	addRoot.Register(srv)

	validate := &ValidateDepsTool{Engine: eng}
	validate.Register(srv)

	logs := &GetInternalLogsTool{Buffer: lb}
	logs.Register(srv)
}
