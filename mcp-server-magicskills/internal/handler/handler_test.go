package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/models"
)

func TestHandleGetSkillWithSection(t *testing.T) {
	eng := engine.NewEngine()
	eng.Skills["test"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test", Description: "test skill"},
		Sections: map[string]string{
			"workflow": "1. Step one\n- Step two",
			"full":     "Full content",
		},
	}

	h := &MagicSkillsHandler{Engine: eng, Logs: &LogBuffer{}}

	// Test valid section
	args := map[string]interface{}{"name": "test", "section": "workflow"}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}

	resp, err := h.HandleGetSkill(context.Background(), req)
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

	h := &MagicSkillsHandler{Engine: eng, Logs: &LogBuffer{}}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"intent": "Searchable"},
		},
	}

	resp, err := h.HandleMatchSkills(context.Background(), req)
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

	h := &MagicSkillsHandler{Engine: eng, Logs: &LogBuffer{}}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"name": "test"},
		},
	}

	resp, err := h.HandleBootstrapTask(context.Background(), req)
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
	// Write over 512KB to trigger truncation.
	// logBufferLimit is 512 * 1024, logTrimTarget is 256 * 1024.
	// We'll write 600KB of 'A's with some newlines.
	chunk := strings.Repeat("A", 1023) + "\n"
	for i := 0; i < 600; i++ {
		lb.Write([]byte(chunk))
	}

	size := len(lb.String())
	if size > logBufferLimit {
		t.Errorf("Expected LogBuffer to truncate to below %d bytes, got %d", logBufferLimit, size)
	}
	if size < logTrimTarget {
		t.Errorf("Expected LogBuffer to keep at least %d bytes, got %d", logTrimTarget, size)
	}
}

func TestHandleReadResource_StatusAndErrors(t *testing.T) {
	eng := engine.NewEngine()
	eng.Skills["test"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test", Version: "1.0"},
	}
	h := &MagicSkillsHandler{Engine: eng, Logs: &LogBuffer{}}

	ctx := context.Background()

	// Test Status
	reqStatus := mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "magicskills://status"},
	}
	res, err := h.HandleReadResource(ctx, reqStatus)
	if err != nil {
		t.Fatalf("HandleReadResource failed: %v", err)
	}
	text := res[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "Total Skills Indexed: 1") {
		t.Fatal("Missing status contents")
	}

	// Test Invalid
	reqInvalid := mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "magicskills://unknown"},
	}
	if _, err := h.HandleReadResource(ctx, reqInvalid); err == nil {
		t.Fatal("Expected error for unknown resource")
	}
}

func TestHandleListResources(t *testing.T) {
	h := &MagicSkillsHandler{}
	res, err := h.HandleListResources(context.Background(), mcp.ListResourcesRequest{})
	if err != nil {
		t.Fatalf("HandleListResources failed: %v", err)
	}
	if len(res.Resources) != 2 {
		t.Fatalf("Expected 2 resources, got %d", len(res.Resources))
	}
}

func TestHandleListSkills(t *testing.T) {
	eng := engine.NewEngine()
	eng.Skills["test"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test", Version: "1.0", Description: "desc"},
	}
	h := &MagicSkillsHandler{Engine: eng}

	res, err := h.HandleListSkills(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("HandleListSkills failed: %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "test") || !strings.Contains(text, "desc") {
		t.Fatal("HandleListSkills missing skill info")
	}
}

func TestHandleGetSkill_EdgeCases(t *testing.T) {
	eng := engine.NewEngine()
	eng.Skills["test"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test", Version: "1.0"},
		Digest:   "Dense Digest",
	}
	h := &MagicSkillsHandler{Engine: eng}
	ctx := context.Background()

	// 1. Missing name
	req1 := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{}}}
	res1, _ := h.HandleGetSkill(ctx, req1)
	if res1.IsError == false {
		t.Fatal("Expected error for missing name")
	}

	// 2. Skill not found
	req2 := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{"name": "unknown"}}}
	res2, _ := h.HandleGetSkill(ctx, req2)
	if res2.IsError == false {
		t.Fatal("Expected error for unknown skill")
	}

	// 3. No section requested (Hybrid test)
	req3 := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{"name": "test"}}}
	res3, _ := h.HandleGetSkill(ctx, req3)
	if len(res3.Content) != 2 {
		t.Fatalf("Expected 2 contents for hybrid result, got %d", len(res3.Content))
	}
	jsonMeta := res3.Content[0].(mcp.TextContent).Text
	if !strings.Contains(jsonMeta, `"name":"test"`) {
		t.Fatal("JSON metadata invalid")
	}
}
