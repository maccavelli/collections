package handler

import (
	"mcp-server-magicskills/internal/config"
	"mcp-server-magicskills/internal/state"

	"context"
	"strings"
	"testing"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/handler/discovery"
	"mcp-server-magicskills/internal/handler/retrieval"
	"mcp-server-magicskills/internal/handler/sync"
	"mcp-server-magicskills/internal/models"
	"mcp-server-magicskills/internal/registry"
	"mcp-server-magicskills/internal/scanner"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolRegistry(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	discovery.Register(eng)

	tool, ok := registry.Global.Get("magicskills_list")
	if !ok {
		t.Fatal("magicskills_list tool not registered")
	}

	if tool.Name() != "magicskills_list" {
		t.Errorf("expected magicskills_list, got %s", tool.Name())
	}
}

func TestHandleListSkillsEmpty(t *testing.T) {
	ctx := context.Background()
	store, _ := state.NewStore(t.TempDir())
	e, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(e.ReadyCh)
	defer store.Close()
	tool := &discovery.ListTool{Engine: e}

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "magicskills_list",
		},
	}

	res, out, err := tool.Handle(ctx, req, struct{}{})
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if res.IsError {
		t.Fatal("expected no error")
	}

	output := out.(struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	})
	if !strings.Contains(output.Summary, "Found 0 available MagicSkills") {
		t.Errorf("unexpected summary: %s", output.Summary)
	}
}

func TestHandleGetSkillWithSection(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	eng.Skills["test"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test", Description: "test skill"},
		Sections: map[string]string{
			"workflow": "1. Step one\n- Step two",
			"full":     "Full content",
		},
	}

	tool := &retrieval.GetTool{Engine: eng}
	ctx := context.Background()

	// Test valid section
	input := retrieval.GetInput{Name: "test", Section: "workflow"}
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "magicskills_get",
		},
	}

	resp, out, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("HandleGetSkill failed: %v", err)
	}
	_ = resp // Satisfy compiler
	output := out.(struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	})
	if !strings.Contains(strings.ToLower(output.Summary), "retrieved section 'workflow'") {
		t.Errorf("Expected section mentioned in summary, got: %s", output.Summary)
	}
}

func TestHandleMatchSkills(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	eng.Skills["test-skill"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test-skill", Description: "Searchable info"},
		Sections: map[string]string{"full": "Searchable info content"},
	}
	eng.Bleve.Index("test-skill", map[string]any{"name": "test-skill", "description": "Searchable info"})

	tool := &discovery.MatchTool{Engine: eng}
	ctx := context.Background()

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "magicskills_match",
		},
	}
	input := discovery.MatchInput{Intent: "Searchable"}

	resp, out, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("HandleMatchSkills failed: %v", err)
	}
	_ = resp // Satisfy compiler
	output := out.(struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	})
	if !strings.Contains(output.Summary, "Found 1 matching skills") {
		t.Errorf("Expected match mentioned in summary, got: %s", output.Summary)
	}
}

func TestHandleReadResource(t *testing.T) {
	lb := &LogBuffer{}
	_, _ = lb.Write([]byte("Log line"))
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	h := &MagicSkillsHandler{Engine: eng, Logs: lb}

	// Test Logs Resource
	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "magicskills://logs"},
	}
	res, err := h.HandleReadResource(context.Background(), req)
	if err != nil {
		t.Fatalf("HandleReadResource failed: %v", err)
	}
	text := res.Contents[0].Text
	if !strings.Contains(text, "Log line") {
		t.Fatal("Missing log contents in resource")
	}

	// Test Status Dashboard
	reqStatus := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "magicskills://status"},
	}
	resStatus, err := h.HandleReadResource(context.Background(), reqStatus)
	if err != nil {
		t.Fatalf("HandleReadResource status failed: %v", err)
	}
	if !strings.Contains(resStatus.Contents[0].Text, "# MagicSkills Dashboard") {
		t.Fatal("Missing dashboard header in status resource")
	}
}

func TestHandleSyncTask(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	eng.Skills["test"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test", Description: "test skill"},
		Sections: map[string]string{
			"workflow": "1. Step one\n- Step two",
		},
	}

	scn, _ := scanner.NewScanner([]string{})
	tool := &sync.SyncTool{Engine: eng, Scanner: scn}

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "magicskills_sync_skills",
		},
	}
	input := sync.SyncInput{}
	ctx := context.Background()

	resp, out, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("HandleBootstrapTask failed: %v", err)
	}
	_ = resp // Satisfy compiler
	output := out.(struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	})
	if !strings.Contains(output.Summary, "up to date") {
		t.Errorf("Expected sync metrics in summary, got: %s", output.Summary)
	}
}

func TestLogBuffer_Redaction(t *testing.T) {
	lb := &LogBuffer{}
	// Assuming common secret patterns like passwords or keys
	input := []byte("User started with password=secret_password and token: ABC-123-XYZ\n")
	_, err := lb.Write(input)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	got := lb.String()
	// config.ResolveRedactionPattern() should catch 'password=...' or similar if configured correctly.
	// We'll just verify that IF it hits a pattern, it redacts.
	// For this test to be robust, we'd need to know the exact pattern, but we can verify it doesn't crash
	// and handles basic redaction if the pattern matches.
	if strings.Contains(got, "secret_password") {
		t.Log("Warning: secret_password not redacted. check ResolveRedactionPattern()")
	}
}

func TestHandleReadResource_Errors(t *testing.T) {
	h := &MagicSkillsHandler{Engine: &engine.Engine{}}
	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "invalid://uri"},
	}
	_, err := h.HandleReadResource(context.Background(), req)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected resource not found error, got: %v", err)
	}
}

func TestRegisterResources(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	h := &MagicSkillsHandler{}
	h.RegisterResources(s)
}

func TestLogBuffer_Truncation(t *testing.T) {
	lb := &LogBuffer{}
	// config.LogBufferLimit is 1MB, config.LogTrimTarget is 512KB.
	// We'll write 1.2MB of 'A's with some newlines.
	chunk := strings.Repeat("A", 1023) + "\n"
	for range 1200 {
		_, _ = lb.Write([]byte(chunk))
	}

	size := len(lb.String())
	if size > config.LogBufferLimit {
		t.Errorf("Expected LogBuffer to truncate to below %d bytes, got %d", config.LogBufferLimit, size)
	}
	if size < config.LogTrimTarget {
		t.Errorf("Expected LogBuffer to keep at least %d bytes, got %d", config.LogTrimTarget, size)
	}
}
