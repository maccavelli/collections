package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/handler/bootstrap"
	"mcp-server-magicskills/internal/handler/discovery"
	"mcp-server-magicskills/internal/handler/retrieval"
	"mcp-server-magicskills/internal/models"
	"mcp-server-magicskills/internal/registry"
)

func TestToolRegistry(t *testing.T) {
	eng := engine.NewEngine()
	discovery.Register(eng)

	tool, ok := registry.Global.Get("magicskills_list")
	if !ok {
		t.Fatal("magicskills_list tool not registered")
	}

	meta := tool.Metadata()
	if meta.Name != "magicskills_list" {
		t.Errorf("expected magicskills_list, got %s", meta.Name)
	}
}

func TestHandleListSkillsEmpty(t *testing.T) {
	ctx := context.Background()
	e := engine.NewEngine()
	tool := &discovery.ListTool{Engine: e}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "magicskills_list",
		},
	}

	res, err := tool.Handle(ctx, req)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Available MagicSkills Index") {
		t.Error("expected index list in output")
	}
}

func TestHandleGetSkillWithSection(t *testing.T) {
	eng := engine.NewEngine()
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
	args := map[string]interface{}{"name": "test", "section": "workflow"}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "magicskills_get",
			Arguments: args,
		},
	}

	resp, err := tool.Handle(ctx, req)
	if err != nil {
		t.Fatalf("HandleGetSkill failed: %v", err)
	}
	text := resp.Content[0].(mcp.TextContent).Text
	if !strings.Contains(strings.ToLower(text), "step one") {
		t.Errorf("Expected step one in output, got: %s", text)
	}
}

func TestHandleMatchSkills(t *testing.T) {
	eng := engine.NewEngine()
	eng.Skills["test-skill"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test-skill", Description: "Searchable info"},
		TermFreq: map[string]int{"searchable": 1, "info": 1},
	}
	eng.RecalculateIndices()

	tool := &discovery.MatchTool{Engine: eng}
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "magicskills_match",
			Arguments: map[string]interface{}{"intent": "Searchable"},
		},
	}

	resp, err := tool.Handle(ctx, req)
	if err != nil {
		t.Fatalf("HandleMatchSkills failed: %v", err)
	}
	text := resp.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "test-skill") {
		t.Errorf("Expected test-skill in search result, got: %s", text)
	}
}

func TestHandleReadResource(t *testing.T) {
	lb := &LogBuffer{}
	_, _ = lb.Write([]byte("Log line"))
	eng := engine.NewEngine()
	h := &MagicSkillsHandler{Engine: eng, Logs: lb}

	// Test Logs Resource
	req := mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "magicskills://logs"},
	}
	res, err := h.HandleReadResource(context.Background(), req)
	if err != nil {
		t.Fatalf("HandleReadResource failed: %v", err)
	}
	text := res[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "Log line") {
		t.Fatal("Missing log contents in resource")
	}
}

func TestHandleBootstrapTask(t *testing.T) {
	eng := engine.NewEngine()
	eng.Skills["test"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test", Description: "test skill"},
		Sections: map[string]string{
			"workflow": "1. Step one\n- Step two",
		},
	}

	tool := &bootstrap.BootstrapTool{Engine: eng}
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "magicskills_bootstrap",
			Arguments: map[string]interface{}{"name": "test"},
		},
	}

	resp, err := tool.Handle(ctx, req)
	if err != nil {
		t.Fatalf("HandleBootstrapTask failed: %v", err)
	}
	text := resp.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "- [ ]") {
		t.Errorf("Expected checklist format - [ ], got: %s", text)
	}
}

func TestLogBuffer_Truncation(t *testing.T) {
	lb := &LogBuffer{}
	// logBufferLimit is 512 * 1024, logTrimTarget is 256 * 1024.
	// We'll write 600KB of 'A's with some newlines.
	chunk := strings.Repeat("A", 1023) + "\n"
	for i := 0; i < 600; i++ {
		_, _ = lb.Write([]byte(chunk))
	}

	size := len(lb.String())
	if size > logBufferLimit {
		t.Errorf("Expected LogBuffer to truncate to below %d bytes, got %d", logBufferLimit, size)
	}
	if size < logTrimTarget {
		t.Errorf("Expected LogBuffer to keep at least %d bytes, got %d", logTrimTarget, size)
	}
}
